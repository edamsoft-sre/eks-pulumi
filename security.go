package main

import (
    "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
    "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateNodeRole creates an IAM role for EKS node instances
func CreateNodeRole(ctx *pulumi.Context) (*iam.Role, error) {
    // Create the IAM role
    nodeRole, err := iam.NewRole(ctx, "node-role", &iam.RoleArgs{
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
        return nil, err
    }

    // Attach the AmazonEKSWorkerNodePolicy
    _, err = iam.NewRolePolicyAttachment(ctx, "node-policy-eks-worker", &iam.RolePolicyAttachmentArgs{
        Role:      nodeRole.Name,
        PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"),
    })
    if err != nil {
        return nil, err
    }

    // Attach the AmazonEC2ContainerRegistryReadOnly policy
    _, err = iam.NewRolePolicyAttachment(ctx, "node-policy-ecr-ro", &iam.RolePolicyAttachmentArgs{
        Role:      nodeRole.Name,
        PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"),
    })
    if err != nil {
        return nil, err
    }

    return nodeRole, nil
}