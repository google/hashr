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

// Package aws implements AWS repository importer.
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
	RepoName     = "AWS"
	buildTimeout = 1800 // 30 minutes
)

// AWS API clients and global configuration vars.
var (
	awsconfig  *aws.Config
	ec2client  *ec2.Client
	s3client   *s3.Client
	bucketname string
	sshuser    string
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

// AwsImage holds data related to AWS image.
type AwsImage struct {
	localImage      types.Image
	sourceImage     types.Image
	localTarGzPath  string
	remoteTarGzPath string
	quickSha256Hash string
	instance        types.Instance
	regionName      string
	volumeId        string
	device          string
	sshclient       *ssh.Client
}

// Preprocess creates tar.gz file from an image, copies to local storage, and extracts it.
func (i *AwsImage) Preprocess() (string, error) {
	var err error

	i.localImage = types.Image{ImageId: aws.String("")}

	i.instance, err = i.getWorkerInstance()
	if err != nil {
		return "", err
	}
	glog.Infof("Using worker instance %s to process %s", *i.instance.InstanceId, *i.sourceImage.ImageId)

	i.sshclient, err = setupSSHClient(sshuser, *i.instance.KeyName, *i.instance.PublicDnsName)
	if err != nil {
		return "", err
	}

	i.regionName, err = i.region()
	if err != nil {
		return "", err
	}

	if err := i.copy(); err != nil {
		return "", err
	}

	if err := i.export(); err != nil {
		return "", err
	}

	if err := i.download(); err != nil {
		return "", fmt.Errorf("error downloading image %s to local storage: %v", *i.sourceImage.ImageId, err)
	}

	if err := i.cleanup(false); err != nil {
		return "", err
	}

	baseDir, _ := filepath.Split(i.localTarGzPath)
	extractionDir := filepath.Join(baseDir, "extracted")

	if err := common.ExtractTarGz(i.localTarGzPath, extractionDir); err != nil {
		return "", fmt.Errorf("error extracting archive %s: %v", i.localTarGzPath, err)
	}

	return filepath.Join(extractionDir, *i.sourceImage.ImageId), nil
}

// ID returns Amazon owned AMI ID.
func (i *AwsImage) ID() string {
	return *i.sourceImage.ImageId
}

// RepoName returns the repository name.
func (i *AwsImage) RepoName() string {
	return RepoName
}

// RepoPath returns the tar.gz image disk in AWS HashR bucket.
func (i *AwsImage) RepoPath() string {
	return i.remoteTarGzPath
}

// LocalPath returns the local path of the AMI tar.gz file.
func (i *AwsImage) LocalPath() string {
	return i.localTarGzPath
}

// ArchiveName returns the tar.gz archive filename of the disk.
func (i *AwsImage) ArchiveName() string {
	return fmt.Sprintf("%s.tar.gz", *i.sourceImage.ImageId)
}

// RemotePath returns disk archive path in AWS.
func (i *AwsImage) RemotePath() string {
	return fmt.Sprintf("s3://%s/%s", bucketname, i.ArchiveName())
}

// Description provides additional description for the Amazon owned AMI.
func (i *AwsImage) Description() string {
	return *i.sourceImage.Description
}

// QuickSHA256Hash returns SHA256 of custom properties of an Amazon owned AMI.
func (i *AwsImage) QuickSHA256Hash() (string, error) {
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
	images   []*AwsImage
}

// NewRepo returns a new instance of AWS repository (Repo).
func NewRepo(ctx context.Context, hashrAwsConfig *aws.Config, hashrEc2Client *ec2.Client, hashrS3Client *s3.Client, hashrBucketName string, hashrSshUser string, osfilter string, osarchs []string) (*Repo, error) {
	glog.Infof("Creating new repo for OS filter %s", osfilter)
	// Setting global variables
	awsconfig = hashrAwsConfig
	ec2client = hashrEc2Client
	s3client = hashrS3Client
	bucketname = hashrBucketName
	sshuser = hashrSshUser

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
	out, err := ec2client.DescribeImages(context.TODO(), &ec2.DescribeImagesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("owner-alias"),
				Values: []string{"amazon"},
			},
			{
				Name:   aws.String("architecture"),
				Values: r.osarchs,
			},
		},
		IncludeDeprecated: aws.Bool(false),
		IncludeDisabled:   aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}

	var sources []hashr.Source

	for _, image := range out.Images {
		if image.Name != nil && strings.Contains(strings.ToLower(*image.Name), strings.ToLower(r.osfilter)) {
			glog.Infof("Discovered %s matching filter %s", *image.ImageId, r.osfilter)

			r.images = append(r.images, &AwsImage{sourceImage: image})
		}
	}

	for _, awsimage := range r.images {
		sources = append(sources, awsimage)
	}

	return sources, nil
}

// getWorkerIntance returns an available instance to process a disk image.
//
// If available instances do not exist, it returns an empty types.Instance and
// error message indicating no free instances.
func (i *AwsImage) getWorkerInstance() (types.Instance, error) {
	diout, err := ec2client.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{})
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

				if err := setInstanceTag(*instance.InstanceId, "InUse", "true"); err != nil {
					glog.Errorf("Error setting tag for instance %s: %v", *instance.InstanceId, err)
				}

				return instance, nil
			}
		}
	}

	return types.Instance{}, fmt.Errorf("unable to find available instance")
}

// region returns AWS availability region for the project.
func (i *AwsImage) region() (string, error) {
	dazOut, err := ec2client.DescribeAvailabilityZones(context.TODO(), &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return "", err
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
func (i *AwsImage) copy() error {
	targetImageName := fmt.Sprintf("copy-%s", *i.sourceImage.ImageId)
	ciout, err := ec2client.CopyImage(context.TODO(), &ec2.CopyImageInput{
		Name:          aws.String(targetImageName),
		SourceImageId: aws.String(*i.sourceImage.ImageId),
		SourceRegion:  aws.String(i.regionName),
	})
	if err != nil {
		return err
	}

	glog.Infof("Copying source image  %s to HashR project image %s", *i.sourceImage.ImageId, *ciout.ImageId)

	for w := 0; w < buildTimeout/100; w++ {
		time.Sleep(30 * time.Second)

		diout, err := ec2client.DescribeImages(context.TODO(), &ec2.DescribeImagesInput{
			ImageIds: []string{*ciout.ImageId},
		})
		if err != nil {
			glog.Errorf("Error getting %s image details. %v", *ciout.ImageId, err)
			return err
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
func (i *AwsImage) export() error {
	if len(i.localImage.BlockDeviceMappings) < 1 {
		return fmt.Errorf("no block device for %s", *i.localImage.ImageId)
	}

	snapshotId := *i.localImage.BlockDeviceMappings[0].Ebs.SnapshotId
	volumeSize := int32(*i.localImage.BlockDeviceMappings[0].Ebs.VolumeSize)
	glog.Infof("Using snapshot %s of the image %s to create volume", snapshotId, *i.sourceImage.ImageId)

	dsout, err := ec2client.DescribeSnapshots(context.TODO(), &ec2.DescribeSnapshotsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("snapshot-id"),
				Values: []string{snapshotId},
			},
		},
	})
	if err != nil {
		glog.Errorf("Error getting details of snapshot %s: %v", snapshotId, err)
		return nil
	}
	snapshot := dsout.Snapshots[0]

	i.volumeId = *snapshot.VolumeId
	if i.volumeId == "vol-ffffffff" {
		glog.Infof("No existing volume for snapshot %s. Creating a new volume...", snapshotId)
		cvout, err := ec2client.CreateVolume(context.TODO(), &ec2.CreateVolumeInput{
			SnapshotId:       aws.String(snapshotId),
			VolumeType:       types.VolumeTypeGp2,
			Size:             aws.Int32(volumeSize),
			AvailabilityZone: aws.String(*i.instance.Placement.AvailabilityZone),
		})
		if err != nil {
			glog.Errorf("Error creating volume from snapshot %s: %v", snapshotId, err)
			return err
		}

		i.volumeId = *cvout.VolumeId
		glog.Infof("Volume %s created from snapshot %s", i.volumeId, snapshotId)
	}

	for w := 0; w < buildTimeout/100; w++ {
		time.Sleep(10 * time.Second)

		dvout, err := ec2client.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("volume-id"),
					Values: []string{i.volumeId},
				},
			},
		})
		if err != nil {
			glog.Errorf("error getting volume details for %s. %v", i.volumeId, err)
			continue
		}

		if dvout.Volumes[0].State == types.VolumeStateAvailable {
			glog.Infof("Volume %s is available for use", i.volumeId)
			break
		}
	}

	i.device, err = getAvailableDevice(i.sshclient)
	if err != nil {
		return err
	}

	glog.Infof("Using %s to attach volume %s on instance %s", i.device, i.volumeId, *i.instance.InstanceId)
	_, err = ec2client.AttachVolume(context.TODO(), &ec2.AttachVolumeInput{
		Device:     aws.String(i.device),
		InstanceId: aws.String(*i.instance.InstanceId),
		VolumeId:   aws.String(i.volumeId),
	})
	if err != nil {
		return err
	}

	volumeAttached := false
	for w := 0; w < buildTimeout/100; w++ {
		time.Sleep(10 * time.Second)
		dvout, err := ec2client.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
			Filters: []types.Filter{
				{
					Name:   aws.String("volume-id"),
					Values: []string{i.volumeId},
				},
			},
		})
		if err != nil {
			glog.Errorf("error getting volume details for %s. %v", i.volumeId, err)
			continue
		}

		for _, attachment := range dvout.Volumes[0].Attachments {
			if attachment.State == types.VolumeAttachmentStateAttached {
				glog.Infof("Volume %s is attached to %s (%s)", i.volumeId, *i.instance.InstanceId, i.device)
				volumeAttached = true
				break
			}
		}
		if volumeAttached {
			break
		}
	}

	if !volumeAttached {
		return fmt.Errorf("error attaching volume %s to instance %s", i.volumeId, *i.instance.InstanceId)
	}

	// Create disk image on a remote EC2 instance
	glog.Infof("Creating disk archive from image %s (volume %s) on instance %s", *i.sourceImage.ImageId, i.volumeId, *i.instance.InstanceId)
	sshcmd := fmt.Sprintf("nohup /usr/local/sbin/hashr-archive %s %s %s &", i.device, *i.sourceImage.ImageId, bucketname)
	_, err = runSshCommand(i.sshclient, sshcmd)
	if err != nil {
		return err
	}

	remoteDoneFile := fmt.Sprintf("/data/%s.done", i.ArchiveName())
	remoteDoneStatus := false

	for w := 0; w < buildTimeout/10; w++ {
		time.Sleep(10 * time.Second)
		statuscmd := fmt.Sprintf("ls %s", remoteDoneFile)
		sshout, err := runSshCommand(i.sshclient, statuscmd)
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

	glog.Infof("Disk archive for %s (%s) is created", *i.sourceImage.ImageId, i.volumeId)
	glog.Infof("Disk archive for %s (%s) is created", *i.sourceImage.ImageId, i.volumeId)
	return nil
}

// cleanup deletes the copied images and done file on HashR worker.
func (i *AwsImage) cleanup(deleteRemotePath bool) error {
	glog.Info("Cleaning up...")
	var err error

	// Delete <image>.tar.gz.done file
	remoteDoneFile := fmt.Sprintf("/data/%s.done", i.ArchiveName())
	sshcmd := fmt.Sprintf("nohup rm -f %s &", remoteDoneFile)
	_, err = runSshCommand(i.sshclient, sshcmd)
	if err != nil {
		glog.Errorf("Error deleting %s on %s: %v", remoteDoneFile, *i.instance.InstanceId, err)
	}

	// Detach and delete volume
	deleteVolume := true
	_, err = ec2client.DetachVolume(context.TODO(), &ec2.DetachVolumeInput{
		VolumeId:   aws.String(i.volumeId),
		Device:     aws.String(i.device),
		InstanceId: aws.String(*i.instance.InstanceId),
	})
	if err != nil {
		glog.Errorf("Error detaching volume %s (%s) from %s: %v", i.volumeId, i.device, *i.instance.InstanceId, err)
		deleteVolume = false
	}

	if deleteVolume {
		ready := false
		for w := 0; w < buildTimeout/100; w++ {
			time.Sleep(10 * time.Second)
			dvout, err := ec2client.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("volume-id"),
						Values: []string{i.volumeId},
					},
				},
			})
			if err != nil {
				glog.Errorf("Getting volume status %s. %v", i.volumeId, err)
				continue
			}

			if dvout.Volumes[0].State == types.VolumeStateAvailable {
				ready = true
				break
			}
		}

		if ready {
			_, err = ec2client.DeleteVolume(context.TODO(), &ec2.DeleteVolumeInput{
				VolumeId: aws.String(i.volumeId),
			})
			if err != nil {
				glog.Errorf("Deleting volume %s. %v", i.volumeId, err)
			}
		}
	}

	// Delete S3 bucket object
	if deleteRemotePath {
		_, err = s3client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: aws.String(bucketname),
			Key:    aws.String(i.ArchiveName()),
		})
		if err != nil {
			glog.Errorf("Deleting %s. %v", i.RemotePath(), err)
		}
	}

	// Update instance tag
	if err := setInstanceTag(*i.instance.InstanceId, "InUse", "false"); err != nil {
		glog.Errorf("Error resetting %s tag. %v", *i.instance.InstanceId, err)
	}
	delete(instanceMap, *i.instance.InstanceId)

	return nil
}

func (i *AwsImage) download() error {
	tempDir, err := common.LocalTempDir(i.ID())
	if err != nil {
		return err
	}
	i.localTarGzPath = filepath.Join(tempDir, i.ArchiveName())

	glog.Infof("Downloading %s to %s", i.RemotePath(), i.localTarGzPath)
	var partMiBs int64 = 10
	downloader := manager.NewDownloader(s3client, func(d *manager.Downloader) {
		d.PartSize = partMiBs * 1024 * 1024
	})

	outFile, err := os.Create(i.localTarGzPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = downloader.Download(context.TODO(), outFile, &s3.GetObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(i.ArchiveName()),
	})
	if err != nil {
		return err
	}

	glog.Infof("Image %s successfully written to %s", *i.sourceImage.ImageId, i.localTarGzPath)

	return nil
}

func setupSSHClient(user string, keyname string, server string) (*ssh.Client, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	key, err := os.ReadFile(filepath.Join(homedir, ".ssh", keyname))
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	sshconfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return ssh.Dial("tcp", fmt.Sprintf("%s:22", server), sshconfig)
}

func runSshCommand(sshclient *ssh.Client, cmd string) (string, error) {
	glog.Infof("Running SSH command %s", cmd)
	session, err := sshclient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf

	if err := session.Run(cmd); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func getAvailableDevice(sshclient *ssh.Client) (string, error) {
	deviceChars := []string{"i", "j", "k", "l", "m", "sdn", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"}
	out, err := runSshCommand(sshclient, "ls /dev/sd* | egrep -v '.*[0-9]$'")
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

func setInstanceTag(instanceId string, tagKey string, tagValue string) error {
	_, err := ec2client.CreateTags(context.TODO(), &ec2.CreateTagsInput{
		Resources: []string{instanceId},
		Tags: []types.Tag{
			{
				Key:   aws.String(tagKey),
				Value: aws.String(tagValue),
			},
		},
	})
	return err
}
