#!/bin/bash
#
# Script to create AWS EC2 instances as HashR workers.
#

# AWS configuration
AWS_PROFILE="default"
AWS_REGION="ap-southeast-2"

# AWS instance source
IMAGE_ID="ami-09eebd0b9bd845bf1"
INSTANCE_TYPE="t2.micro"
INSTANCE_COUNT=2
KEY_NAME="HashrAwsKey"
USER="ec2-user"
USER_DATA="file://hashr_aws_init.txt"
WORKER_TAG_INUSE_NAME="InUse"
WORKER_TAG_INUSE_VALUE="false"
WORKER_TAG_ROLE_NAME="role"
WORKER_TAG_ROLE_VALUE="hashr-worker"

SECURITY_GROUP_NAME="hashr-security-group"
SECURITY_GROUP_ID=""

# NOTE: You should change this to limit exposure.
SECURITY_SOURCE_CIDR="0.0.0.0/0"
WORKER_AWS_CONFIG_FILE="hashr.uploader.tar.gz"

SCRIPT_DIR=`dirname $0`
logfile=${SCRIPT_DIR}/hashr_aws_setup.log
touch $logfile

create_key_pair() {
    local keyPairId

   echo "Creating AWS key pair ${KEY_NAME}" 

    keyPairId=`aws --profile ${AWS_PROFILE} ec2 describe-key-pairs --filters Name=key-name,Values=${KEY_NAME} | jq -r '.KeyPairs[0].KeyPairId'`
    if [ "${keyPairId}" == "null" ]; then
        aws --profile ${AWS_PROFILE} ec2 create-key-pair --key-name ${KEY_NAME} | jq -r '.KeyMaterial' > $HOME/.ssh/${KEY_NAME}
        chmod 600 ${HOME}/.ssh/${KEY_NAME}

        keyPairId=`aws --profile ${AWS_PROFILE} ec2 describe-key-pairs --filters Name=key-name,Values=${KEY_NAME} | jq -r '.KeyPairs[0].KeyPairId'`
        echo -e "  - Created a new AWS key pair ${keyPairId}"
        return
    fi

    echo -e "  Key pair ${KEY_NAME} exists with ID ${keyPairId}"
}

create_security_group_id() {
    local securityGroupId

    echo "Setting up security group ${SECURITY_GROUP_NAME}"

    SECURITY_GROUP_ID=`aws --profile ${AWS_PROFILE} ec2 describe-security-groups --filters Name=group-name,Values=${SECURITY_GROUP_NAME} | jq -r '.SecurityGroups[].GroupId'`
    if [ "${SECURITY_GROUP_ID}" == "" ]; then
        securityGroupId=`aws --profile ${AWS_PROFILE} ec2 create-security-group --group-name ${SECURITY_GROUP_NAME} --description "Security group for HashR AWS worker" | jq -r '.GroupId'`
        aws --profile ${AWS_PROFILE} ec2 authorize-security-group-ingress --group-id ${securityGroupId} --protocol tcp --port 22 --cidr "${SECURITY_SOURCE_CIDR}" > $logfile 2>&1
 
        SECURITY_GROUP_ID=${securityGroupId}
        sleep 5

        echo -e "  - Created security group ${SECURITY_GROUP_NAME} (${securityGroupId})"
    else
        echo -e "  - Security group ${SECURITY_GROUP_NAME} exists ${SECURITY_GROUP_ID}"
    fi
}

check_instance_status() {
    local instanceId="$1"
    local instanceState="$2"
    local instanceStateName=""

    local count=0
    while true
    do
        if [ $count -ge 5 ]; then
            echo "Something went wrong. The instance $instanceId should be up by now"
            return 1
        fi

        instanceStateName=`aws --profile ${AWS_PROFILE} ec2 describe-instances --instance-ids ${instanceId} | jq -r '.Reservations[].Instances[0].State.Name'`
        echo "  Current state of ${instanceId} is ${instanceStateName}"
        if [ "${instanceStateName}" == "${instanceState}" ]; then
            return 0
        fi

        sleep 10   
        count=$((count + 1))
    done
}

copy_aws_config() {
    local instanceId="$1"
    local publicDnsName=""
    local sshOptions="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
    local securityGroupname=""

    echo "  - Copying AWS configuration to instance ${instanceId}"
    securityGroupName=`aws --profile ${AWS_PROFILE} ec2 describe-instance-attribute --instance-id ${instanceId} --attribute groupSet | jq -r '.Groups[].GroupName'`
    if [ "$securityGroupName}" != "${SECURITY_GROUP_NAME}" ]; then
        aws --profile ${AWS_PROFILE} ec2 modify-instance-attribute --instance-id ${instanceId} --groups ${SECURITY_GROUP_ID}
        sleep 5
    fi

    publicDnsName=`aws --profile ${AWS_PROFILE} ec2 describe-instances --instance-id ${instanceId} | jq -r '.Reservations[].Instances[0].PublicDnsName'` 
    scp -i ~/.ssh/${KEY_NAME} ${sshOptions} ${SCRIPT_DIR}/${WORKER_AWS_CONFIG_FILE} ${USER}@${publicDnsName}:~/ > $logfile 2>&1
    ssh -i ~/.ssh/${KEY_NAME} ${sshOptions} ${USER}@${publicDnsName} "tar -zxf ~/${WORKER_AWS_CONFIG_FILE} -C ~/" > $logfile 2>&1
}

run_ec2_instance() {
    local instanceSateName=""
    local volumeId=""

    echo "Running ${INSTANCE_COUNT} EC2 instances"

    for instanceId in `aws --profile ${AWS_PROFILE} ec2 run-instances --image-id ${IMAGE_ID} --count ${INSTANCE_COUNT} --instance-type ${INSTANCE_TYPE} --key-name ${KEY_NAME} --security-group-ids ${SECURITY_GROUP_ID} --associate-public-ip-address --tag-specifications 'ResourceType=instance,Tags=[{Key=role,Value=hashr-worker},{Key=InUse,Value=false},]' --user-data ${USER_DATA} | jq -r '.Instances[].InstanceId'`
    do
        # We want to make sure the instance is in the running state before we increase
        # the size of the disk.
        echo "  - Checking if ${instanceId} is running"
        check_instance_status ${instanceId} "running"
        if [ $? -eq 1 ]; then
          exit 1
        fi

        # Increase the size of the disk.
        volumeId=`aws --profile ${AWS_PROFILE} ec2 describe-volumes --filters Name=attachment.instance-id,Values=i-094382052d8b0c550 Name=attachment.device,Values=/dev/xvda | jq -r '.Volumes[0].VolumeId'`
        aws --profile ${AWS_PROFILE} ec2 modify-volume --volume-id ${volumeId} --size 50 > $logfile 2>&1

        # We need to restart the instance to take effect of the new disk size.
        aws --profile ${AWS_PROFILE} ec2 stop-instances --instance-id ${instanceId} > $logfile 2>&1
        echo "  - Checking if ${instanceId} is stopped"
        check_instance_status ${instanceId} "stopped"
        if [ $? -eq 1 ]; then
            exit 1
        fi

        aws --profile ${AWS_PROFILE} ec2 start-instances --instance-id ${instanceId} > $logfile 2>&1
        echo "  - Checking if ${instanceId} is running"
        check_instance_status ${instanceId} "running"
        if [ $? -eq 1 ]; then
            exit 1
        fi

        copy_aws_config ${instanceId}

        echo -e "  - Created HashR worker ${instanceId}"
    done
}

remove_key_pair() {
    echo "Removing key pair ${KEY_NAME}"
    aws --profile ${AWS_PROFILE} ec2 delete-key-pair --key-name ${KEY_NAME}
}

remove_security_group() {
    local securityGroupId

    securityGroupId=`aws --profile ${AWS_PROFILE} ec2 describe-security-groups --filters Name=group-name,Values=${SECURITY_GROUP_NAME} | jq -r '.SecurityGroups[].GroupId'`
    echo "Security group ID ${securityGroupId} for ${SECURITY_GROUP_NAME}"

    if [ "${securityGroupId}" == "" ]; then
        echo "  - No security group ID for security group ${SECURITY_GROUP_NAME}"
    else
        # Check if security-group-id is still in use
        instances=`aws --profile ${AWS_PROFILE} ec2 describe-instances --filters Name=instance.group-id,Values=${securityGroupId} | jq -r '.Reservations[].Instances[].InstanceId'`
        if [ "${instances}" != "" ]; then
            echo -e "Security group ${securityGroupId} (${SECURITY_GROUP_NAME}) is in use in the following instances:\n${instances}"
        else
            # Delete security group.
            echo "Removing security group ${SECURITY_GROUP_NAME} (${securityGroupId})"
            aws --profile ${AWS_PROFILE} ec2 delete-security-group --group-id ${securityGroupId}
        fi
    fi
}

remove_instances() {
    echo "Removing EC2 worker instances"

    for instanceId in `aws --profile ${AWS_PROFILE} ec2 describe-instances --filters Name=tag-value,Values=${WORKER_TAG_ROLE_VALUE} | jq -r '.Reservations[].Instances[].InstanceId'`
    do
        echo "  - Removing the worker instance ${instanceId}"
        aws --profile ${AWS_PROFILE} ec2 terminate-instances --instance-id ${instanceId} > $logfile 2>&1
    done
}

# Main
case "$1" in 
    setup)
        dirpath=`dirname $0`
        if [ ! -f ${dirpath}/${WORKER_AWS_CONFIG_FILE} ]; then
            echo "No AWS configuration file (${WORKER_AWS_CONFIG_FILE}) for worker"
            exit 1
        fi

        create_key_pair

        sleep 5
        create_security_group_id

        sleep 5
        run_ec2_instance
        ;;
    create-key)
        echo "Creating keypair ${KEY_NAME}"
        create_key_pair
        ;;
    create-sg)
        echo "Creating security group ${SECURITY_GROUP_NAME}"
        create_security_group_id
        ;;
    remove-key)
        echo "Removing key pair ${KEY_NAME}"
        remove_key_pair
        ;;
    remove-sg)
        echo "Removing security group ${SECURITY_GROUP_NAME}"
        remove_security_group
        ;;
    remove-instance)
        echo "Removing EC2 worker instances"
        remove_instances
        ;;
    remove-all)
        echo "Removing HashR AWS instances, security group, and key pair"
        remove_instances

        sleep 5
        remove_security_group

        sleep 5
        remove_key_pair
        ;;
    *)
        echo "Usage: `basename $0` {setup|create-key|create-sg|remove-key|remove-sg|remove-instance|remove-all}" || true
        exit 1
esac

exit 0
