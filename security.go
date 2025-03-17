package main

import (
	"fmt"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func CreateNodeResources(ctx *pulumi.Context) (*ec2.Vpc, *iam.Role, []pulumi.StringOutput, error) {
	// Create IAM role
	nodeRole, err := iam.NewRole(ctx, ClusterName+"-"+"node-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
            "Version": "2012-10-17",
            "Statement": [{
                "Effect": "Allow",
                "Principal": {"Service": "ec2.amazonaws.com"},
                "Action": "sts:AssumeRole"
            }]
        }`),
		Description: pulumi.String("IAM role for EKS node group"),
	})
	if err != nil {
		ctx.Log.Error("Failed to create node role", &pulumi.LogArgs{})
		return nil, nil, nil, err
	}

	_, err = iam.NewRolePolicyAttachment(ctx, ClusterName+"-eks-worker", &iam.RolePolicyAttachmentArgs{
		Role:      nodeRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"),
	})
	if err != nil {
		ctx.Log.Error("Failed to attach EKS worker policy", &pulumi.LogArgs{})
		return nil, nil, nil, err
	}
	
    _, err = iam.NewRolePolicyAttachment(ctx, ClusterName+"-vpc-controller", &iam.RolePolicyAttachmentArgs{
		Role:      nodeRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKSVPCResourceController"),
	})
	if err != nil {
		ctx.Log.Error("Failed to attach VPC controller policy", &pulumi.LogArgs{})
		return nil, nil, nil, err
	}

	_, err = iam.NewRolePolicyAttachment(ctx, ClusterName+"-ecr-ro", &iam.RolePolicyAttachmentArgs{
		Role:      nodeRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"),
	})
	if err != nil {
		ctx.Log.Error("Failed to attach ECR policy", &pulumi.LogArgs{})
		return nil, nil, nil, err
	}	
    
    _, err = iam.NewRolePolicyAttachment(ctx, ClusterName+"-eks-cni", &iam.RolePolicyAttachmentArgs{
		Role:      nodeRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"),
	})
	if err != nil {
		ctx.Log.Error("Failed to attach EKS CNI policy", &pulumi.LogArgs{})
		return nil, nil, nil, err
	}
    

	// Create VPC
	ctx.Log.Info("Creating VPC", nil)
	vpc, err := ec2.NewVpc(ctx, ClusterName+"eks-vpc", &ec2.VpcArgs{
		CidrBlock: pulumi.String("172.27.0.0/16"),
        EnableDnsHostnames: pulumi.Bool(true),
        EnableDnsSupport: pulumi.Bool(true),

	})
	if err != nil {
		ctx.Log.Error("Failed to create VPC", &pulumi.LogArgs{})
		return nil, nil, nil, err
	}

	// Define subnet configurations
	subnetConfigs := []struct {
		cidr string
		az   string
	}{
		{"172.27.0.0/27", Region+"a"},
		{"172.27.0.32/27", Region+"b"},
	}

	// Create private subnets
	privateSubnets := make([]pulumi.StringOutput, 0, len(subnetConfigs))
	for i, config := range subnetConfigs {
		name := fmt.Sprintf("%s-%d", ClusterName, i+1)
		ctx.Log.Info(fmt.Sprintf("Creating subnet: %s with CIDR %s in AZ %s", name, config.cidr, config.az), nil)
		subnet, err := ec2.NewSubnet(ctx, name, &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(config.cidr),
			AvailabilityZone: pulumi.String(config.az),
		})
		if err != nil {
			ctx.Log.Error(fmt.Sprintf("Failed to create subnet %s", name), &pulumi.LogArgs{})
			return nil, nil, nil, err
		}
		privateSubnets = append(privateSubnets, subnet.ID().ToStringOutput())
	}

    

	if len(privateSubnets) == 0 {
		ctx.Log.Error("No subnets were created", nil)
		return nil, nil, nil, fmt.Errorf("no subnets created")
	}

	ctx.Log.Info(fmt.Sprintf("Successfully created %d subnets", len(privateSubnets)), nil)

	return vpc, nodeRole, privateSubnets, nil

}


