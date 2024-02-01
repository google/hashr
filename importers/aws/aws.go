// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package AWS implements AWS repository importer.
package aws

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"
	"golang.org/x/crypto/ssh"
)

const (
	// RepoName contains the importer repository name.
	RepoName     = "AWS"
	buildTimeout = 1800 // 30 minutes
)

// AWS API clients and global configuration vars.
var (
	ec2Client  *ec2.Client
	s3Client   *s3.Client
	bucketName string
	sshUser    string
)

// instanceMap stores the state of the available EC2 HashR worker instances.
//
// If an EC2 instance is currently processing an AMI, instanceMap is set where
// instance ID is the key and the AMI ID being processed is the value.
//
// If a key (instance ID) does not exist, it means it is not in use.
var instanceMap map[string]string

func init() {
	instanceMap = make(map[string]string)
}

// image holds data related to AWS image.
type image struct {
	localImage      types.Image
	sourceImage     types.Image
	localTarGzPath  string
	remoteTarGzPath string
	quickSha256Hash string
	instance        types.Instance
	regionName      string
	volumeID        string
	device          string
	sshClient       *ssh.Client
}

// Preprocess creates tar.gz file from an image, copies to local storage, and extracts it.
func (i *image) Preprocess() (string, error) {
	var err error
	ctx := context.TODO()

	i.localImage = types.Image{ImageId: aws.String("")}

	i.instance, err = i.getWorkerInstance(ctx)
	if err != nil {
		return "", fmt.Errorf("error getting worker instances: %v", err)
	}
	glog.Infof("Using worker instance %s to process %s", *i.instance.InstanceId, *i.sourceImage.ImageId)

	i.sshClient, err = setupsshClient(sshUser, *i.instance.KeyName, *i.instance.PublicDnsName)
	if err != nil {
		return "", fmt.Errorf("error setting up SSH client: %v", err)
	}

	i.regionName, err = i.region(ctx)
	if err != nil {
		return "", fmt.Errorf("error getting EC2 region name: %v", err)
	}

	if err := i.copy(ctx); err != nil {
		return "", fmt.Errorf("error copying AMI %s to AWS HashR project: %v", *i.sourceImage.ImageId, err)
	}

	if err := i.export(ctx); err != nil {
		return "", fmt.Errorf("error exporting disk image of AMI %s: %v", *i.sourceImage.ImageId, err)
	}

	if err := i.download(ctx); err != nil {
		return "", fmt.Errorf("error downloading image %s to local storage: %v", *i.sourceImage.ImageId, err)
	}

	if err := i.cleanup(ctx, false); err != nil {
		return "", fmt.Errorf("error cleaning up post-processing: %v", err)
	}

	baseDir, _ := filepath.Split(i.localTarGzPath)
	extractionDir := filepath.Join(baseDir, "extracted")

	if err := common.ExtractTarGz(i.localTarGzPath, extractionDir); err != nil {
		return "", fmt.Errorf("error extracting archive %s: %v", i.localTarGzPath, err)
	}

	return filepath.Join(extractionDir, *i.sourceImage.ImageId), nil
}

// ID returns Amazon owned AMI ID.
func (i *image) ID() string {
	// Replacing spaces, (, and ) with _.
	imageName := strings.Replace(*i.sourceImage.ImageLocation, "amazon", "", -1)
	imageName = strings.Replace(imageName, "/", "_", -1)
	imageName = strings.Replace(imageName, " ", "_", -1)
	imageName = strings.Replace(imageName, "(", "_", -1)
	imageName = strings.Replace(imageName, ")", "_", -1)

	return fmt.Sprintf("%s_%s", *i.sourceImage.ImageId, imageName)
}

// RepoName returns the repository name.
func (i *image) RepoName() string {
	return RepoName
}

// RepoPath returns the tar.gz image disk in AWS HashR bucket.
func (i *image) RepoPath() string {
	return i.remoteTarGzPath
}

// LocalPath returns the local path of the AMI tar.gz file.
func (i *image) LocalPath() string {
	return i.localTarGzPath
}

// ArchiveName returns the tar.gz archive filename of the disk.
func (i *image) ArchiveName() string {
	return fmt.Sprintf("%s.tar.gz", *i.sourceImage.ImageId)
}

// RemotePath returns disk archive path in AWS.
func (i *image) RemotePath() string {
	return fmt.Sprintf("s3://%s/%s", bucketName, i.ArchiveName())
}

// Description provides additional description for the Amazon owned AMI.
func (i *image) Description() string {
	return *i.sourceImage.Description
}

// QuickSHA256Hash returns SHA256 of custom properties of an Amazon owned AMI.
func (i *image) QuickSHA256Hash() (string, error) {
	if i.quickSha256Hash != "" {
		return i.quickSha256Hash, nil
	}

	data := [][]byte{
		[]byte(*i.sourceImage.ImageId),
		[]byte("|"),
		[]byte(*i.sourceImage.ImageLocation),
		[]byte("|"),
		[]byte(*i.sourceImage.CreationDate),
		[]byte("|"),
		[]byte(*i.sourceImage.DeprecationTime),
	}

	var hashBytes []byte

	for _, bytes := range data {
		hashBytes = append(hashBytes, bytes...)
	}

	i.quickSha256Hash = fmt.Sprintf("%x", sha256.Sum256(hashBytes))
	return i.quickSha256Hash, nil
}

// Repo holds data related to AWS repository.
type Repo struct {
	osfilter string
	osarchs  []string
	images   []*image
}

// NewRepo returns a new instance of AWS repository (Repo).
func NewRepo(ctx context.Context, hashrEc2Client *ec2.Client, hashrS3Client *s3.Client, hashrBucketName string, hashrSSHUser string, osfilter string, osarchs []string) (*Repo, error) {
	glog.Infof("Creating new repo for OS filter %s", osfilter)
	// Setting global variables
	ec2Client = hashrEc2Client
	s3Client = hashrS3Client
	bucketName = hashrBucketName
	sshUser = hashrSSHUser

	return &Repo{
		osfilter: osfilter,
		osarchs:  osarchs,
	}, nil
}

// RepoName returns AWS repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns the Amazon AMI OS filters.
func (r *Repo) RepoPath() string {
	return r.osfilter
}

// DiscoverRepo returns a list of AMI matching the AMI filters.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	var sources []hashr.Source

	images, err := getAmazonImages(context.TODO(), ec2Client, r.osarchs)
	if err != nil {
		return nil, err
	}

	for _, i := range images {
		if i.Name != nil && strings.Contains(strings.ToLower(*i.Name), strings.ToLower(r.osfilter)) {
			glog.Infof("Discovered %s matching filter %s", *i.ImageId, r.osfilter)

			r.images = append(r.images, &image{sourceImage: i})
		}
	}

	for _, i := range r.images {
		sources = append(sources, i)
	}

	return sources, nil
}

// getWorkerIntance returns an available instance to process a disk image.
//
// If available instances do not exist, it returns an empty types.Instance and
// error message indicating no free instances.
func (i *image) getWorkerInstance(ctx context.Context) (types.Instance, error) {
	diout, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
		},
	})
	if err != nil {
		return types.Instance{}, err
	}

	for _, reservation := range diout.Reservations {
		for _, instance := range reservation.Instances {
			if instanceInUse(instance) {
				glog.Infof("Instance %s is in use", *instance.InstanceId)
				continue
			}

			_, ok := instanceMap[*instance.InstanceId]
			if !ok {
				instanceMap[*instance.InstanceId] = *i.sourceImage.ImageId

				if err := setInstanceTag(ctx, *instance.InstanceId, "InUse", "true"); err != nil {
					glog.Errorf("Error setting tag for instance %s: %v", *instance.InstanceId, err)
				}

				return instance, nil
			}
		}
	}

	return types.Instance{}, fmt.Errorf("unable to find available instance")
}

// region returns AWS availability region for the project.
func (i *image) region(ctx context.Context) (string, error) {
	dazOut, err := ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return "", fmt.Errorf("error getting availability zone: %v", err)
	}

	var regionName string

	for _, zone := range dazOut.AvailabilityZones {
		if zone.RegionName != nil {
			regionName = *zone.RegionName
			return regionName, nil
		}
	}

	return "", fmt.Errorf("error getting region name")
}

// copy copies Amazon owned AMI to HashR project.
//
// Note: This operation will create a new AMI on HashR project.
// Disk image is created based on the snapshot in AMI in the HashR project.
func (i *image) copy(ctx context.Context) error {
	targetImageName := fmt.Sprintf("copy-%s", *i.sourceImage.ImageId)
	ciout, err := ec2Client.CopyImage(ctx, &ec2.CopyImageInput{
		Name:          aws.String(targetImageName),
		SourceImageId: aws.String(*i.sourceImage.ImageId),
		SourceRegion:  aws.String(i.regionName),
	})
	if err != nil {
		return fmt.Errorf("error running AWS API CopyImage: %v", err)
	}

	glog.Infof("Copying source image  %s to HashR project image %s", *i.sourceImage.ImageId, *ciout.ImageId)

	for w := 0; w < buildTimeout/100; w++ {
		time.Sleep(30 * time.Second)

		diout, err := ec2Client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			ImageIds: []string{*ciout.ImageId},
		})
		if err != nil {
			glog.Errorf("Error getting %s image details. %v", *ciout.ImageId, err)
			return fmt.Errorf("error running AWS API DescribeImages: %v", err)
		}

		i.localImage = diout.Images[0]

		if diout.Images[0].State == types.ImageStateAvailable {
			break
		}
	}

	if *i.localImage.ImageId == "" {
		return fmt.Errorf("unable to get image Id")
	}
	return nil
}

// export exports a tar.gz disk image to AWS S3 bucket.
func (i *image) export(ctx context.Context) error {
	if len(i.localImage.BlockDeviceMappings) < 1 {
		return fmt.Errorf("no block device for %s", *i.localImage.ImageId)
	}

	snapshotID := *i.localImage.BlockDeviceMappings[0].Ebs.SnapshotId
	volumeSize := int32(*i.localImage.BlockDeviceMappings[0].Ebs.VolumeSize)
	glog.Infof("Using snapshot %s of the image %s to create volume", snapshotID, *i.sourceImage.ImageId)

	dsout, err := ec2Client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("snapshot-id"),
				Values: []string{snapshotID},
			},
		},
	})
	if err != nil {
		glog.Errorf("Error getting details of snapshot %s: %v", snapshotID, err)
		return nil
	}
	snapshot := dsout.Snapshots[0]

	i.volumeID = *snapshot.VolumeId
	if i.volumeID == "vol-ffffffff" {
		glog.Infof("No existing volume for snapshot %s. Creating a new volume...", snapshotID)
		cvout, err := ec2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
			SnapshotId:       aws.String(snapshotID),
			VolumeType:       types.VolumeTypeGp2,
			Size:             aws.Int32(volumeSize),
			AvailabilityZone: aws.String(*i.instance.Placement.AvailabilityZone),
		})
		if err != nil {
			glog.Errorf("Error creating volume from snapshot %s: %v", snapshotID, err)
			return fmt.Errorf("error running AWS API CreateVolume: %v", err)
		}

		i.volumeID = *cvout.VolumeId
		glog.Infof("Volume %s created from snapshot %s", i.volumeID, snapshotID)
	}

	for w := 0; w < buildTimeout/100; w++ {
		time.Sleep(10 * time.Second)

		dvout, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("volume-id"),
					Values: []string{i.volumeID},
				},
			},
		})
		if err != nil {
			glog.Errorf("error getting volume details for %s. %v", i.volumeID, err)
			continue
		}

		if dvout.Volumes[0].State == types.VolumeStateAvailable {
			glog.Infof("Volume %s is available for use", i.volumeID)
			break
		}
	}

	i.device, err = getAvailableDevice(i.sshClient)
	if err != nil {
		return fmt.Errorf("error getting available device on a remote worker: %v", err)
	}

	glog.Infof("Using %s to attach volume %s on instance %s", i.device, i.volumeID, *i.instance.InstanceId)
	_, err = ec2Client.AttachVolume(ctx, &ec2.AttachVolumeInput{
		Device:     aws.String(i.device),
		InstanceId: aws.String(*i.instance.InstanceId),
		VolumeId:   aws.String(i.volumeID),
	})
	if err != nil {
		return fmt.Errorf("error running AWS API AttachVolume: %v", err)
	}

	volumeAttached := false
	for w := 0; w < buildTimeout/100; w++ {
		time.Sleep(10 * time.Second)
		dvout, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("volume-id"),
					Values: []string{i.volumeID},
				},
			},
		})
		if err != nil {
			glog.Errorf("error getting volume details for %s. %v", i.volumeID, err)
			continue
		}

		for _, attachment := range dvout.Volumes[0].Attachments {
			if attachment.State == types.VolumeAttachmentStateAttached {
				glog.Infof("Volume %s is attached to %s (%s)", i.volumeID, *i.instance.InstanceId, i.device)
				volumeAttached = true
				break
			}
		}
		if volumeAttached {
			break
		}
	}

	if !volumeAttached {
		return fmt.Errorf("error attaching volume %s to instance %s", i.volumeID, *i.instance.InstanceId)
	}

	// Create disk image on a remote EC2 instance
	glog.Infof("Creating disk archive from image %s (volume %s) on instance %s", *i.sourceImage.ImageId, i.volumeID, *i.instance.InstanceId)
	sshcmd := fmt.Sprintf("nohup /usr/local/sbin/hashr-archive %s %s %s &", i.device, *i.sourceImage.ImageId, bucketName)
	_, err = runSSHCommand(i.sshClient, sshcmd)
	if err != nil {
		return fmt.Errorf("error running SSH command: %v", err)
	}

	remoteDoneFile := fmt.Sprintf("/data/%s.done", i.ArchiveName())
	remoteDoneStatus := false

	for w := 0; w < buildTimeout/10; w++ {
		time.Sleep(10 * time.Second)
		statuscmd := fmt.Sprintf("ls %s", remoteDoneFile)
		sshout, err := runSSHCommand(i.sshClient, statuscmd)
		if err != nil {
			glog.Errorf("error executing remote command %s: %v", statuscmd, err)
		}

		if strings.Contains(sshout, remoteDoneFile) {
			remoteDoneStatus = true
			break
		}
	}

	if !remoteDoneStatus {
		return fmt.Errorf("disk archive not created within %d seconds", buildTimeout)
	}

	glog.Infof("Disk archive for %s (%s) is created", *i.sourceImage.ImageId, i.volumeID)
	glog.Infof("Disk archive for %s (%s) is created", *i.sourceImage.ImageId, i.volumeID)
	return nil
}

// cleanup deletes the copied images and done file on HashR worker.
func (i *image) cleanup(ctx context.Context, deleteRemotePath bool) error {
	glog.Info("Cleaning up...")
	var err error

	// Delete <image>.tar.gz.done file
	remoteDoneFile := fmt.Sprintf("/data/%s.done", i.ArchiveName())
	sshcmd := fmt.Sprintf("nohup rm -f %s &", remoteDoneFile)
	_, err = runSSHCommand(i.sshClient, sshcmd)
	if err != nil {
		glog.Errorf("Error deleting %s on %s: %v", remoteDoneFile, *i.instance.InstanceId, err)
	}

	// Detach and delete volume
	deleteVolume := true
	_, err = ec2Client.DetachVolume(ctx, &ec2.DetachVolumeInput{
		VolumeId:   aws.String(i.volumeID),
		Device:     aws.String(i.device),
		InstanceId: aws.String(*i.instance.InstanceId),
	})
	if err != nil {
		glog.Errorf("Error detaching volume %s (%s) from %s: %v", i.volumeID, i.device, *i.instance.InstanceId, err)
		deleteVolume = false
	}

	if deleteVolume {
		ready := false
		for w := 0; w < buildTimeout/100; w++ {
			time.Sleep(10 * time.Second)
			dvout, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("volume-id"),
						Values: []string{i.volumeID},
					},
				},
			})
			if err != nil {
				glog.Errorf("Getting volume status %s. %v", i.volumeID, err)
				continue
			}

			if dvout.Volumes[0].State == types.VolumeStateAvailable {
				ready = true
				break
			}
		}

		if ready {
			_, err = ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
				VolumeId: aws.String(i.volumeID),
			})
			if err != nil {
				glog.Errorf("Deleting volume %s. %v", i.volumeID, err)
			}
		}
	}

	// Delete S3 bucket object
	if deleteRemotePath {
		_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(i.ArchiveName()),
		})
		if err != nil {
			glog.Errorf("Deleting %s. %v", i.RemotePath(), err)
		}
	}

	// Update instance tag
	if err := setInstanceTag(ctx, *i.instance.InstanceId, "InUse", "false"); err != nil {
		glog.Errorf("Error resetting %s tag. %v", *i.instance.InstanceId, err)
	}
	delete(instanceMap, *i.instance.InstanceId)

	return nil
}

func (i *image) download(ctx context.Context) error {
	tempDir, err := common.LocalTempDir(i.ID())
	if err != nil {
		return fmt.Errorf("error creating a local temporary directory: %v", err)
	}
	i.localTarGzPath = filepath.Join(tempDir, i.ArchiveName())

	glog.Infof("Downloading %s to %s", i.RemotePath(), i.localTarGzPath)
	var partMiBs int64 = 10
	downloader := manager.NewDownloader(s3Client, func(d *manager.Downloader) {
		d.PartSize = partMiBs * 1024 * 1024
	})

	outFile, err := os.Create(i.localTarGzPath)
	if err != nil {
		return fmt.Errorf("error creating local tar.gz path: %v", err)
	}
	defer outFile.Close()

	_, err = downloader.Download(ctx, outFile, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(i.ArchiveName()),
	})
	if err != nil {
		return fmt.Errorf("error downloading disk archive from S3 bucket: %v", err)
	}

	glog.Infof("Image %s successfully written to %s", *i.sourceImage.ImageId, i.localTarGzPath)

	return nil
}

func setupsshClient(user string, keyname string, server string) (*ssh.Client, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting home directory: %v", err)
	}

	key, err := os.ReadFile(filepath.Join(homedir, ".ssh", keyname))
	if err != nil {
		return nil, fmt.Errorf("error reading SSH key pair: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("error parsing SSH key pair: %v", err)
	}

	sshconfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return ssh.Dial("tcp", fmt.Sprintf("%s:22", server), sshconfig)
}

func runSSHCommand(sshClient *ssh.Client, cmd string) (string, error) {
	glog.Infof("Running SSH command %s", cmd)
	session, err := sshClient.NewSession()
	if err != nil {
		return "", fmt.Errorf("error setting a new SSH client session: %v", err)
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("error running remote SSH command: %v", err)
	}

	return buf.String(), nil
}

func getAvailableDevice(sshClient *ssh.Client) (string, error) {
	deviceChars := []string{"i", "j", "k", "l", "m", "sdn", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"}
	out, err := runSSHCommand(sshClient, "ls /dev/sd* | egrep -v '.*[0-9]$'")
	if err != nil {
		return "/dev/sdh", nil
	}

	usedDevices := strings.Split(out, "\n")
	for _, deviceChar := range deviceChars {
		used := false
		device := fmt.Sprintf("/dev/sd%s", deviceChar)
		for _, usedDevice := range usedDevices {
			if usedDevice == device {
				used = true
				break
			}
		}

		if !used {
			return device, nil
		}
	}

	return "", fmt.Errorf("no free device available for volume attachment")
}

func instanceInUse(instance types.Instance) bool {
	for _, tag := range instance.Tags {
		if *tag.Key == "InUse" && *tag.Value == "true" {
			return true
		}
	}
	return false
}

func setInstanceTag(ctx context.Context, instanceID string, tagKey string, tagValue string) error {
	_, err := ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{instanceID},
		Tags: []types.Tag{
			{
				Key:   aws.String(tagKey),
				Value: aws.String(tagValue),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("error creating tags on instances: %v", err)
	}

	return nil
}

type ec2DescribeImagesAPI interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

func getAmazonImages(ctx context.Context, api ec2DescribeImagesAPI, architectures []string) ([]types.Image, error) {
	out, err := api.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("owner-alias"),
				Values: []string{"amazon"},
			},
			{
				Name:   aws.String("architecture"),
				Values: architectures,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error getting Amazon public images: %v", err)
	}

	return out.Images, nil
}
