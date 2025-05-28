package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
)

func setup(t *testing.T) (context.Context, *awsHandler, *MockAWSClientsBuilderInterface, *MockEC2ClientInterface, func()) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	builderMock := NewMockAWSClientsBuilderInterface(ctrl)
	clientMock := NewMockEC2ClientInterface(ctrl)
	handler := &awsHandler{
		staticCredentials: credentials.NewStaticCredentialsProvider("ak", "sk", ""),
		logger:            logr.Discard(),
		clientsBuilder:    builderMock,
	}
	return ctx, handler, builderMock, clientMock, func() { ctrl.Finish() }
}

// RunInstanceInRegion
func TestRunInstanceInRegion_Success(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	params := &RunInstanceParams{
		Region:          "us-east-1",
		InstanceType:    "t2.micro",
		AMI:             "ami-123",
		KeyPair:         "keypair",
		SecurityGroupID: "sg-1",
		SubnetID:        "subnet-1",
		UserData:        "user-data-base64",
		PoolName:        "mypool",
		Device: BlockDeviceSpec{
			DeviceName: "/dev/sdf",
			DeviceSize: 20,
			DeviceType: string(ec2types.VolumeTypeGp2),
		},
	}

	builder.
		EXPECT().GetEC2Client(ctx, params.Region, handler.staticCredentials).
		Return(client, nil)
	client.
		EXPECT().RunInstances(ctx, gomock.Any()).
		Return(&ec2.RunInstancesOutput{Instances: []ec2types.Instance{{InstanceId: lo.ToPtr("i-abc")}}}, nil)

	id, err := handler.RunInstanceInRegion(ctx, params)
	assert.NoError(t, err)
	assert.Equal(t, "i-abc", *id)
}

func TestRunInstanceInRegion_ErrorScenarios(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	// Builder error
	params1 := &RunInstanceParams{Region: "region1"}
	builder.
		EXPECT().GetEC2Client(ctx, "region1", handler.staticCredentials).
		Return(nil, errors.New("build fail"))
	_, err := handler.RunInstanceInRegion(ctx, params1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create EC2 client")

	// API error
	params2 := &RunInstanceParams{Region: "region2"}
	builder.
		EXPECT().GetEC2Client(ctx, "region2", handler.staticCredentials).
		Return(client, nil)
	client.
		EXPECT().RunInstances(ctx, gomock.Any()).
		Return(nil, errors.New("api failed"))
	_, err = handler.RunInstanceInRegion(ctx, params2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed running instance")

	// No instances returned
	params3 := &RunInstanceParams{Region: "region3"}
	builder.
		EXPECT().GetEC2Client(ctx, "region3", handler.staticCredentials).
		Return(client, nil)
	client.
		EXPECT().RunInstances(ctx, gomock.Any()).
		Return(&ec2.RunInstancesOutput{}, nil)
	_, err = handler.RunInstanceInRegion(ctx, params3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no instance ID returned")
}

// ReleaseInstanceInRegion
func TestReleaseInstanceInRegion_Success(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	ident := &InstanceIdentifier{Region: "us-west-2", InstanceID: "i-abc"}
	builder.
		EXPECT().GetEC2Client(ctx, ident.Region, handler.staticCredentials).
		Return(client, nil)
	client.
		EXPECT().TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{ident.InstanceID}}).
		Return(&ec2.TerminateInstancesOutput{}, nil)

	err := handler.ReleaseInstanceInRegion(ctx, ident)
	assert.NoError(t, err)
}

func TestReleaseInstanceInRegion_Error(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	ident := &InstanceIdentifier{Region: "us-west-2", InstanceID: "i-abc"}
	builder.
		EXPECT().GetEC2Client(ctx, ident.Region, handler.staticCredentials).
		Return(client, nil)
	client.
		EXPECT().TerminateInstances(ctx, gomock.Any()).
		Return(nil, errors.New("terminate fail"))

	err := handler.ReleaseInstanceInRegion(ctx, ident)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "terminate fail")
}

// FindInstanceRegion
func TestFindInstanceRegion_Success(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	builder := NewMockAWSClientsBuilderInterface(ctrl)
	client1 := NewMockEC2ClientInterface(ctrl)
	client2 := NewMockEC2ClientInterface(ctrl)
	h := &awsHandler{staticCredentials: credentials.NewStaticCredentialsProvider("ak", "sk", ""), logger: logr.Discard(), clientsBuilder: builder}

	// r1: NotFound, r2: success
	builder.EXPECT().GetEC2Client(ctx, "r1", h.staticCredentials).Return(client1, nil)
	client1.EXPECT().DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{"i-1"}}).
		Return(nil, &smithy.GenericAPIError{Code: "NotFound"})

	builder.EXPECT().GetEC2Client(ctx, "r2", h.staticCredentials).Return(client2, nil)
	client2.EXPECT().DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{"i-1"}}).
		Return(&ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{InstanceId: lo.ToPtr("i-1")}}}}}, nil)

	region, err := h.FindInstanceRegion(ctx, &FindRegionParams{InstanceID: "i-1", PossibleRegions: []string{"r1", "r2"}})
	assert.NoError(t, err)
	assert.Equal(t, "r2", *region)
}

func TestFindInstanceRegion_AllFail(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	builder := NewMockAWSClientsBuilderInterface(ctrl)
	h := &awsHandler{staticCredentials: credentials.NewStaticCredentialsProvider("ak", "sk", ""), logger: logr.Discard(), clientsBuilder: builder}

	builder.EXPECT().GetEC2Client(ctx, "r1", h.staticCredentials).Return(nil, errors.New("b1"))
	builder.EXPECT().GetEC2Client(ctx, "r2", h.staticCredentials).Return(nil, errors.New("b2"))

	region, err := h.FindInstanceRegion(ctx, &FindRegionParams{InstanceID: "i-x", PossibleRegions: []string{"r1", "r2"}})
	assert.Error(t, err)
	assert.Nil(t, region)
}

// GetTotalAmountOfPoolInstancesInRegion
func TestGetTotalAmountOfPoolInstancesInRegion_Success(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	params := &PoolFilterParams{Region: "us-west-1", PoolName: "mypool"}
	builder.EXPECT().GetEC2Client(ctx, params.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(&ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{}}, {Instances: []ec2types.Instance{{}, {}}}},
	}, nil)

	total, err := handler.GetTotalAmountOfPoolInstancesInRegion(ctx, params)
	assert.NoError(t, err)
	assert.Equal(t, 2, *total)
}

func TestGetTotalAmountOfPoolInstancesInRegion_Error(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	params := &PoolFilterParams{Region: "us-west-1", PoolName: "mypool"}
	builder.EXPECT().GetEC2Client(ctx, params.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(nil, errors.New("list error"))

	_, err := handler.GetTotalAmountOfPoolInstancesInRegion(ctx, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

// IsInstanceInRegionActive
func TestIsInstanceInRegionActive_Success(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	ident := &InstanceIdentifier{Region: "eu-central-1", InstanceID: "i-run"}
	builder.EXPECT().GetEC2Client(ctx, ident.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(&ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{State: &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning}}}}},
	}, nil)

	active, err := handler.IsInstanceInRegionActive(ctx, ident)
	assert.NoError(t, err)
	assert.True(t, *active)
}

func TestIsInstanceInRegionActive_ErrorScenarios(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	ident1 := &InstanceIdentifier{Region: "regionA", InstanceID: "i-miss"}
	builder.EXPECT().GetEC2Client(ctx, ident1.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(&ec2.DescribeInstancesOutput{}, nil)
	_, err := handler.IsInstanceInRegionActive(ctx, ident1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "doesn't exist")

	ident2 := &InstanceIdentifier{Region: "regionB", InstanceID: "i-nostate"}
	builder.EXPECT().GetEC2Client(ctx, ident2.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(&ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{State: nil}}}}}, nil)
	_, err = handler.IsInstanceInRegionActive(ctx, ident2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "doesn't have a state")
}

// GetInstanceInRegionPublicIP
func TestGetInstanceInRegionPublicIP_Success(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	ident := &InstanceIdentifier{Region: "ap-south-1", InstanceID: "iip"}
	builder.EXPECT().GetEC2Client(ctx, ident.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(&ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{PublicIpAddress: lo.ToPtr("1.2.3.4")}}}},
	}, nil)

	ip, err := handler.GetInstanceInRegionPublicIP(ctx, ident)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", *ip)
}

func TestGetInstanceInRegionPublicIP_ErrorScenarios(t *testing.T) {
	ctx, handler, builder, client, teardown := setup(t)
	defer teardown()

	ident := &InstanceIdentifier{Region: "cn-north-1", InstanceID: "i-ip"}
	builder.EXPECT().GetEC2Client(ctx, ident.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(nil, errors.New("desc error"))
	_, err := handler.GetInstanceInRegionPublicIP(ctx, ident)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "desc error")

	builder.EXPECT().GetEC2Client(ctx, ident.Region, handler.staticCredentials).Return(client, nil)
	client.EXPECT().DescribeInstances(ctx, gomock.Any()).Return(&ec2.DescribeInstancesOutput{}, nil)
	_, err = handler.GetInstanceInRegionPublicIP(ctx, ident)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "doesn't exist")
}

func TestAWSProviderFactory_DefaultConfig(t *testing.T) {
	// No secretData => missing creds error
	_, err := AWSProviderFactory("", map[string][]byte{}, logr.Discard())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestAWSProviderFactory_BadJSON(t *testing.T) {
	bad := map[string][]byte{"config": []byte(`{"invalid_json":`)}
	_, err := AWSProviderFactory("", bad, logr.Discard())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error in provider config json")
}

func TestAWSProviderFactory_BadUserData(t *testing.T) {
	// invalid base64 userdata
	cfg := `{"accessKey":"ak","secretAccessKey":"sk","userdata":"!!not-base64!!"}`
	secret := map[string][]byte{"config": []byte(cfg)}
	_, err := AWSProviderFactory("", secret, logr.Discard())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error decoding userdata")
}

func TestAcquire_SuccessAndErrors(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	awsHandlerMock := NewMockAWSHandlerInterface(ctrl)
	spec := awsProviderConfig{
		AccessKey:       "ak",
		SecretAccessKey: "sk",
		MachineSpec: &MachineSpec{
			Regions: []RegionSpec{{
				Name:            "us-west-1",
				KeyPairName:     "kp",
				SecurityGroupID: "sg",
				SubnetID:        "subnet-1",
				Instances:       []InstanceSpec{{Type: "t1", AMIID: "ami1"}},
			}},
			Device: BlockDeviceSpec{DeviceName: "/dev/xvda", DeviceSize: 8, DeviceType: "gp2"},
		},
	}
	prov := &awsProvider{config: spec, logger: logr.Discard(), handler: awsHandlerMock}

	// 1) Error counting existing instances
	awsHandlerMock.EXPECT().GetTotalAmountOfPoolInstancesInRegion(
		ctx,
		&PoolFilterParams{Region: "us-west-1", PoolName: "poolA"},
	).Return(nil, errors.New("count fail"))
	_, err := prov.Acquire(1, "poolA", "ptype")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed counting the total amount")

	// 2) Already at capacity
	awsHandlerMock.EXPECT().GetTotalAmountOfPoolInstancesInRegion(
		ctx,
		&PoolFilterParams{Region: "us-west-1", PoolName: "poolA"},
	).Return(lo.ToPtr(2), nil)
	_, err = prov.Acquire(1, "poolA", "ptype")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to create instance")

	// 3) RunInstanceInRegion failure
	awsHandlerMock.EXPECT().GetTotalAmountOfPoolInstancesInRegion(
		ctx,
		&PoolFilterParams{Region: "us-west-1", PoolName: "poolA"},
	).Return(lo.ToPtr(0), nil)
	awsHandlerMock.EXPECT().RunInstanceInRegion(
		ctx,
		&RunInstanceParams{
			Region:          "us-west-1",
			InstanceType:    "t1",
			AMI:             "ami1",
			KeyPair:         "kp",
			SecurityGroupID: "sg",
			SubnetID:        "subnet-1",
			UserData:        "",
			PoolName:        "poolA",
			Device:          spec.MachineSpec.Device,
		},
	).Return(nil, errors.New("launch fail"))
	_, err = prov.Acquire(1, "poolA", "ptype")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error creating instance")

	// 4) Success path
	resID := "i-xyz"
	awsHandlerMock.EXPECT().GetTotalAmountOfPoolInstancesInRegion(
		ctx,
		&PoolFilterParams{Region: "us-west-1", PoolName: "poolA"},
	).Return(lo.ToPtr(0), nil)
	awsHandlerMock.EXPECT().RunInstanceInRegion(
		ctx,
		&RunInstanceParams{
			Region:          "us-west-1",
			InstanceType:    "t1",
			AMI:             "ami1",
			KeyPair:         "kp",
			SecurityGroupID: "sg",
			SubnetID:        "subnet-1",
			UserData:        "",
			PoolName:        "poolA",
			Device:          spec.MachineSpec.Device,
		},
	).Return(lo.ToPtr(resID), nil)

	res, err := prov.Acquire(1, "poolA", "ptype")
	assert.NoError(t, err)
	assert.Equal(t, resID, res.Id)
}

func TestAcquireCompleted_Scenarios(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	awsHandlerMock := NewMockAWSHandlerInterface(ctrl)
	prov := &awsProvider{
		config:  awsProviderConfig{MachineSpec: &MachineSpec{Regions: []RegionSpec{{Name: "us-east-1"}}}},
		handler: awsHandlerMock,
		logger:  logr.Discard(),
	}
	id := "i-1"

	// 1) FindInstanceRegion error
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-east-1"}},
	).Return(nil, errors.New("find err"))
	ok, _, err := prov.AcquireCompleted(id)
	assert.False(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error finding instance")

	// 2) Active check error
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-east-1"}},
	).Return(lo.ToPtr("us-east-1"), nil)
	awsHandlerMock.EXPECT().IsInstanceInRegionActive(
		ctx,
		&InstanceIdentifier{Region: "us-east-1", InstanceID: id},
	).Return(nil, errors.New("state err"))
	ok, _, err = prov.AcquireCompleted(id)
	assert.False(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking if instance is active")

	// 3) Not running
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-east-1"}},
	).Return(lo.ToPtr("us-east-1"), nil)
	awsHandlerMock.EXPECT().IsInstanceInRegionActive(
		ctx,
		&InstanceIdentifier{Region: "us-east-1", InstanceID: id},
	).Return(lo.ToPtr(false), nil)
	ok, res, err := prov.AcquireCompleted(id)
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, id, res.Id)

	// 4) Public IP error
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-east-1"}},
	).Return(lo.ToPtr("us-east-1"), nil)
	awsHandlerMock.EXPECT().IsInstanceInRegionActive(
		ctx,
		&InstanceIdentifier{Region: "us-east-1", InstanceID: id},
	).Return(lo.ToPtr(true), nil)
	awsHandlerMock.EXPECT().GetInstanceInRegionPublicIP(
		ctx,
		&InstanceIdentifier{Region: "us-east-1", InstanceID: id},
	).Return(nil, errors.New("ip err"))
	ok, _, err = prov.AcquireCompleted(id)
	assert.False(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error checking if instance public IP")

	// 5) Success
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-east-1"}},
	).Return(lo.ToPtr("us-east-1"), nil)
	awsHandlerMock.EXPECT().IsInstanceInRegionActive(
		ctx,
		&InstanceIdentifier{Region: "us-east-1", InstanceID: id},
	).Return(lo.ToPtr(true), nil)
	awsHandlerMock.EXPECT().GetInstanceInRegionPublicIP(
		ctx,
		&InstanceIdentifier{Region: "us-east-1", InstanceID: id},
	).Return(lo.ToPtr("1.2.3.4"), nil)
	ok, res, err = prov.AcquireCompleted(id)
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "1.2.3.4", res.Address)
}

func TestRelease_Scenarios(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	awsHandlerMock := NewMockAWSHandlerInterface(ctrl)
	prov := &awsProvider{
		config:  awsProviderConfig{MachineSpec: &MachineSpec{Regions: []RegionSpec{{Name: "us-west-2"}}}},
		handler: awsHandlerMock,
		logger:  logr.Discard(),
	}
	id := "i-1"

	// 1) FindInstanceRegion error
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-west-2"}},
	).Return(nil, errors.New("find err"))
	err := prov.Release(id)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error finding instance for release")

	// 2) ReleaseInstanceInRegion error
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-west-2"}},
	).Return(lo.ToPtr("us-west-2"), nil)
	awsHandlerMock.EXPECT().ReleaseInstanceInRegion(
		ctx,
		&InstanceIdentifier{Region: "us-west-2", InstanceID: id},
	).Return(errors.New("rel fail"))
	err = prov.Release(id)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error releasing instance")

	// 3) Success
	awsHandlerMock.EXPECT().FindInstanceRegion(
		ctx,
		&FindRegionParams{InstanceID: id, PossibleRegions: []string{"us-west-2"}},
	).Return(lo.ToPtr("us-west-2"), nil)
	awsHandlerMock.EXPECT().ReleaseInstanceInRegion(
		ctx,
		&InstanceIdentifier{Region: "us-west-2", InstanceID: id},
	).Return(nil)
	err = prov.Release(id)
	assert.NoError(t, err)
}
