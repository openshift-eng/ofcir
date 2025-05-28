package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
)

//go:generate mockgen -source=aws.go -destination=mock_aws.go -package=providers

type EC2ClientInterface interface {
	RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

type AWSClientsBuilderInterface interface {
	GetEC2Client(ctx context.Context, region string, staticCredentials credentials.StaticCredentialsProvider) (EC2ClientInterface, error)
}

type AWSClientsBuilder struct{}

func (c *AWSClientsBuilder) GetEC2Client(ctx context.Context, region string, staticCredentials credentials.StaticCredentialsProvider) (EC2ClientInterface, error) {
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithCredentialsProvider(staticCredentials),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}

const (
	MinInstanceCount = 1
	MaxInstanceCount = 1

	PoolNameFilter          = "tag:cipool"
	InstanceStateFilterName = "instance-state-name"
	RunningState            = "running"
)

var _ AWSHandlerInterface = &awsHandler{}

type AWSHandlerInterface interface {
	RunInstanceInRegion(ctx context.Context, params *RunInstanceParams) (*string, error)
	ReleaseInstanceInRegion(ctx context.Context, params *InstanceIdentifier) error
	FindInstanceRegion(ctx context.Context, params *FindRegionParams) (*string, error)
	GetTotalAmountOfPoolInstancesInRegion(ctx context.Context, params *PoolFilterParams) (*int, error)
	IsInstanceInRegionActive(ctx context.Context, params *InstanceIdentifier) (*bool, error)
	GetInstanceInRegionPublicIP(ctx context.Context, params *InstanceIdentifier) (*string, error)
}

type RunInstanceParams struct {
	Region          string
	InstanceType    string
	AMI             string
	KeyPair         string
	SecurityGroupID string
	SubnetID        string
	UserData        string
	PoolName        string
	Device          BlockDeviceSpec
}

type InstanceIdentifier struct {
	Region     string
	InstanceID string
}

type FindRegionParams struct {
	InstanceID      string
	PossibleRegions []string
}

type PoolFilterParams struct {
	Region   string
	PoolName string
}

type awsHandler struct {
	staticCredentials credentials.StaticCredentialsProvider
	logger            logr.Logger
	clientsBuilder    AWSClientsBuilderInterface
}

func NewAWSHandler(
	awsAccessKey, awsSecretAccessKey string,
	logger logr.Logger,
) (*awsHandler, error) {
	if awsAccessKey == "" || awsSecretAccessKey == "" {
		return nil, errors.New("AWS access key and secret access key are required")
	}

	return &awsHandler{
		staticCredentials: credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretAccessKey, ""),
		logger:            logger,
		clientsBuilder:    &AWSClientsBuilder{},
	}, nil
}

func (h *awsHandler) RunInstanceInRegion(ctx context.Context, params *RunInstanceParams) (*string, error) {
	client, err := h.clientsBuilder.GetEC2Client(ctx, params.Region, h.staticCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to create EC2 client for region %s: %w", params.Region, err)
	}

	input := &ec2.RunInstancesInput{
		ImageId:          aws.String(params.AMI),
		InstanceType:     ec2types.InstanceType(params.InstanceType),
		KeyName:          aws.String(params.KeyPair),
		MinCount:         aws.Int32(MinInstanceCount),
		MaxCount:         aws.Int32(MaxInstanceCount),
		SecurityGroupIds: []string{params.SecurityGroupID},
		SubnetId:         aws.String(params.SubnetID),
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{
			{
				DeviceName: aws.String(params.Device.DeviceName),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize:          aws.Int32(params.Device.DeviceSize),
					VolumeType:          ec2types.VolumeType(params.Device.DeviceType),
					DeleteOnTermination: aws.Bool(true),
				},
			},
		},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String("cipool"), Value: aws.String(params.PoolName)},
				},
			},
		},
	}
	if params.UserData != "" {
		input.UserData = aws.String(params.UserData)
	}

	output, err := client.RunInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf(
			"failed running instance %s with ami ID %s in region %s with keypair name %s, security group %s, subnet ID %s and block devices %s: %w",
			params.InstanceType,
			params.AMI,
			params.Region,
			params.KeyPair,
			params.SecurityGroupID,
			params.SubnetID,
			fmt.Sprintf("%+v", params.Device),
			err,
		)
	}

	if len(output.Instances) == 0 || output.Instances[0].InstanceId == nil {
		return nil, fmt.Errorf("no instance ID returned")
	}

	return output.Instances[0].InstanceId, nil
}

func (h *awsHandler) ReleaseInstanceInRegion(
	ctx context.Context,
	instanceIdentifier *InstanceIdentifier,
) error {
	client, err := h.clientsBuilder.GetEC2Client(ctx, instanceIdentifier.Region, h.staticCredentials)
	if err != nil {
		return fmt.Errorf("failed to create EC2 client for region %s: %w", instanceIdentifier.Region, err)
	}

	_, err = client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceIdentifier.InstanceID},
	})
	if err != nil {
		return fmt.Errorf("failed to terminate instance %s in region %s: %w",
			instanceIdentifier.InstanceID,
			instanceIdentifier.Region,
			err,
		)
	}

	return nil
}

func (h *awsHandler) FindInstanceRegion(ctx context.Context, params *FindRegionParams) (*string, error) {
	for _, region := range params.PossibleRegions {
		client, err := h.clientsBuilder.GetEC2Client(ctx, region, h.staticCredentials)
		if err != nil {
			continue
		}

		output, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: []string{params.InstanceID},
		})
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) && strings.Contains(apiErr.ErrorCode(), "NotFound") {
				continue
			}
			return nil, fmt.Errorf("failed describing instances in region %s: %w", region, err)
		}

		if len(output.Reservations) > 0 && len(output.Reservations[0].Instances) > 0 {
			return lo.ToPtr(region), nil
		}
	}

	return nil, fmt.Errorf("instance %s not found in any region", params.InstanceID)
}

func (h *awsHandler) GetTotalAmountOfPoolInstancesInRegion(ctx context.Context, params *PoolFilterParams) (*int, error) {
	client, err := h.clientsBuilder.GetEC2Client(ctx, params.Region, h.staticCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to create EC2 client for region %s: %w", params.Region, err)
	}

	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{Name: lo.ToPtr(PoolNameFilter), Values: []string{params.PoolName}},
			{Name: lo.ToPtr(InstanceStateFilterName), Values: []string{RunningState}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing all running instances from pool %s in %s: %w", params.PoolName, params.Region, err)
	}

	total := 0
	for _, res := range out.Reservations {
		total += len(res.Instances)
	}

	return lo.ToPtr(total), nil
}

func (h *awsHandler) IsInstanceInRegionActive(ctx context.Context, instanceIdentifier *InstanceIdentifier) (*bool, error) {
	client, err := h.clientsBuilder.GetEC2Client(ctx, instanceIdentifier.Region, h.staticCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to create EC2 client for region %s: %w", instanceIdentifier.Region, err)
	}

	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceIdentifier.InstanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed describing instance %s in region %s: %w", instanceIdentifier.InstanceID, instanceIdentifier.Region, err)
	}

	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance %s in region %s doesn't exist", instanceIdentifier.InstanceID, instanceIdentifier.Region)
	}

	instance := out.Reservations[0].Instances[0]
	if instance.State == nil {
		return nil, fmt.Errorf("instance %s in region %s doesn't have a state", instanceIdentifier.InstanceID, instanceIdentifier.Region)
	}

	return lo.ToPtr(instance.State.Name == RunningState), nil
}

func (h *awsHandler) GetInstanceInRegionPublicIP(ctx context.Context, instanceIdentifier *InstanceIdentifier) (*string, error) {
	client, err := h.clientsBuilder.GetEC2Client(ctx, instanceIdentifier.Region, h.staticCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to create EC2 client for region %s: %w", instanceIdentifier.Region, err)
	}

	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceIdentifier.InstanceID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed describing instance %s in region %s: %w", instanceIdentifier.InstanceID, instanceIdentifier.Region, err)
	}

	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance %s in region %s doesn't exist", instanceIdentifier.InstanceID, instanceIdentifier.Region)
	}

	if out.Reservations[0].Instances[0].PublicIpAddress == nil {
		return nil, fmt.Errorf("instance %s in region %s doesn't have public IP address", instanceIdentifier.InstanceID, instanceIdentifier.Region)
	}

	return out.Reservations[0].Instances[0].PublicIpAddress, nil
}

const (
	defaultInstanceType = "c5n.metal"
	defaultAMI          = "ami-0a73e96a849c232cc" // Rocky 9.5 x86_64 in us-east-1
	defaultRegion       = "us-east-1"
	defaultDeviceName   = "/dev/xvda"
	defaultDeviceSize   = 1024  // GiB
	defaultDeviceType   = "gp2" // General purpose SSD
)

type InstanceSpec struct {
	Type  string `json:"type"`
	AMIID string `json:"amiID"`
}

type RegionSpec struct {
	Name            string         `json:"name"`
	KeyPairName     string         `json:"keyPairName"`
	SecurityGroupID string         `json:"securityGroupID"`
	SubnetID        string         `json:"subnetID"`
	Instances       []InstanceSpec `json:"instances"`
}

type BlockDeviceSpec struct {
	DeviceName string `json:"deviceName"` // logical device name e.g. "/dev/xvda"
	DeviceSize int32  `json:"deviceSize"` // in GiB
	DeviceType string `json:"deviceType"` // e.g. gp2, gp3, etc.
}

type MachineSpec struct {
	Regions []RegionSpec    `json:"regions"`
	Device  BlockDeviceSpec `json:"device"`
}

type awsProviderConfig struct {
	AccessKey       string       `json:"accessKey"`
	SecretAccessKey string       `json:"secretAccessKey"`
	UserData        string       `json:"userData,omitempty"`
	MachineSpec     *MachineSpec `json:"machineSpec"`
}

type awsProvider struct {
	config  awsProviderConfig
	logger  logr.Logger
	handler AWSHandlerInterface
}

func AWSProviderFactory(providerInfo string, secretData map[string][]byte, logger logr.Logger) (Provider, error) {
	config := awsProviderConfig{
		MachineSpec: &MachineSpec{
			Regions: []RegionSpec{
				{
					Name: defaultRegion,
					Instances: []InstanceSpec{
						{Type: defaultInstanceType, AMIID: defaultAMI},
					},
				},
			},
			Device: BlockDeviceSpec{
				DeviceName: defaultDeviceName,
				DeviceSize: defaultDeviceSize,
				DeviceType: defaultDeviceType,
			},
		},
	}

	if configJson, ok := secretData["config"]; ok {
		if err := json.Unmarshal(configJson, &config); err != nil {
			return nil, fmt.Errorf("error in provider config json: %w", err)
		}
	}

	if config.UserData != "" {
		// Provided userdata from secret should be base64 encoded
		decodedBytes, err := base64.StdEncoding.DecodeString(config.UserData)
		if err != nil {
			return nil, fmt.Errorf("error decoding userdata: %w", err)
		}
		config.UserData = string(decodedBytes)
	}

	awsHandler, err := NewAWSHandler(config.AccessKey, config.SecretAccessKey, logger)
	if err != nil {
		return nil, fmt.Errorf("failed creating AWS handler: %w", err)
	}

	return &awsProvider{
		config:  config,
		logger:  logger,
		handler: awsHandler,
	}, nil
}

func (p *awsProvider) Acquire(poolSize int, poolName string, poolType string) (Resource, error) {
	ctx := context.Background()

	total := 0
	for _, regionSpec := range p.config.MachineSpec.Regions {
		count, err := p.handler.GetTotalAmountOfPoolInstancesInRegion(
			ctx, &PoolFilterParams{Region: regionSpec.Name, PoolName: poolName},
		)
		if err != nil {
			return Resource{}, fmt.Errorf(
				"failed counting the total amount of instances in pool %s in region %s: %w", poolName, regionSpec.Name, err,
			)
		}
		total += lo.FromPtr(count)
	}

	if total >= poolSize {
		return Resource{}, fmt.Errorf(
			"refusing to create instance, already have %d and pool size is %d", total, poolSize,
		)
	}

	for _, regionSpec := range p.config.MachineSpec.Regions {
		for _, instanceSpec := range regionSpec.Instances {
			id, err := p.handler.RunInstanceInRegion(
				ctx,
				&RunInstanceParams{
					Region:          regionSpec.Name,
					InstanceType:    instanceSpec.Type,
					AMI:             instanceSpec.AMIID,
					KeyPair:         regionSpec.KeyPairName,
					SecurityGroupID: regionSpec.SecurityGroupID,
					SubnetID:        regionSpec.SubnetID,
					UserData:        p.config.UserData,
					PoolName:        poolName,
					Device: BlockDeviceSpec{
						DeviceName: p.config.MachineSpec.Device.DeviceName,
						DeviceSize: p.config.MachineSpec.Device.DeviceSize,
						DeviceType: p.config.MachineSpec.Device.DeviceType,
					},
				},
			)
			if err != nil {
				p.logger.Error(err, "error running instance")
				continue
			}

			return Resource{Id: lo.FromPtr(id)}, nil
		}
	}

	return Resource{}, errors.New("error creating instance")
}

func (p *awsProvider) AcquireCompleted(id string) (bool, Resource, error) {
	ctx := context.Background()
	res := Resource{Id: id}

	region, err := p.handler.FindInstanceRegion(
		ctx, &FindRegionParams{InstanceID: id, PossibleRegions: p.getSupportedRegions()},
	)
	if err != nil {
		return false, res, fmt.Errorf("error finding instance: %w", err)
	}

	isRunning, err := p.handler.IsInstanceInRegionActive(
		ctx, &InstanceIdentifier{Region: lo.FromPtr(region), InstanceID: id},
	)
	if err != nil {
		return false, res, fmt.Errorf("error checking if instance is active: %w", err)
	}

	if !lo.FromPtr(isRunning) {
		return false, res, nil
	}

	ip, err := p.handler.GetInstanceInRegionPublicIP(
		ctx, &InstanceIdentifier{Region: lo.FromPtr(region), InstanceID: id},
	)
	if err != nil {
		return false, res, fmt.Errorf("error checking if instance public IP: %w", err)
	}

	res.Address = lo.FromPtr(ip)

	return true, res, nil
}

func (p *awsProvider) Clean(id string) error {
	// AWS doesn't support reloading instance
	return nil
}

func (p *awsProvider) CleanCompleted(id string) (bool, error) {
	// AWS doesn't support reloading instance
	return true, nil
}

func (p *awsProvider) Release(id string) error {
	ctx := context.Background()

	region, err := p.handler.FindInstanceRegion(
		ctx, &FindRegionParams{InstanceID: id, PossibleRegions: p.getSupportedRegions()},
	)
	if err != nil {
		return fmt.Errorf("error finding instance for release: %w", err)
	}

	if err := p.handler.ReleaseInstanceInRegion(
		ctx, &InstanceIdentifier{Region: lo.FromPtr(region), InstanceID: id},
	); err != nil {
		return fmt.Errorf("error releasing instance: %w", err)
	}

	return nil
}

func (p *awsProvider) getSupportedRegions() []string {
	return lo.Map(p.config.MachineSpec.Regions, func(regionSpec RegionSpec, _ int) string {
		return regionSpec.Name
	})
}
