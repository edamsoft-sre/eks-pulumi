package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const Name = "gore"
const identifier = "100"
const ClusterName = Name + "-" + identifier
const VpcName = ClusterName
const InstanceType = "t3.small" //"t3.medium"
var AZ = []string{"a", "b"}     // EKS requires 2 AZs
const Region = "us-east-1"

func main() {

	pulumi.Run(func(ctx *pulumi.Context) error {

		vpc, nodeRole, privateSubnets, err := CreateNodeResources(ctx)
		if err != nil {
			ctx.Log.Error("CreateNodeResources failed", &pulumi.LogArgs{})
			return err
		}
		if len(privateSubnets) == 0 {
			ctx.Log.Error("No subnets returned from CreateNodeResources", nil)
			return fmt.Errorf("no private subnets available")
		}

		ec2Endpoint, err := ec2.NewVpcEndpoint(ctx, ClusterName+"-"+"ec2-endpoint", &ec2.VpcEndpointArgs{
			VpcId:             vpc.ID(),
			ServiceName:       pulumi.String("com.amazonaws."+Region+".ec2"),
			VpcEndpointType:   pulumi.String("Interface"),
			SubnetIds:         pulumi.ToStringArrayOutput(privateSubnets),
			//SecurityGroupIds: pulumi.StringArray{cluster.ClusterSecurityGroupId},
			PrivateDnsEnabled: pulumi.Bool(true),
		})
		
		if err != nil {
			return err
		}
		ctx.Export("ec2EndpointUrl", ec2Endpoint.Arn)

		eksEndpoint, err := ec2.NewVpcEndpoint(ctx, ClusterName+"-"+"eks-endpoint", &ec2.VpcEndpointArgs{
			VpcId:             vpc.ID(),
			ServiceName:       pulumi.String("com.amazonaws."+Region+".eks"),
			VpcEndpointType:   pulumi.String("Interface"),
			SubnetIds:         pulumi.ToStringArrayOutput(privateSubnets),
			//SecurityGroupIds: pulumi.StringArray{cluster.ClusterSecurityGroupId},
			PrivateDnsEnabled: pulumi.Bool(true),
		})
		
		if err != nil {
			return err
		}
		ctx.Export("eksEndpointUrl", eksEndpoint.Arn)

		ecrEndpoint, err := ec2.NewVpcEndpoint(ctx, ClusterName+"-"+"ecr-endpoint", &ec2.VpcEndpointArgs{
			VpcId:             vpc.ID(),
			ServiceName:       pulumi.String("com.amazonaws."+Region+".ecr.api"),
			VpcEndpointType:   pulumi.String("Interface"),
			SubnetIds:         pulumi.ToStringArrayOutput(privateSubnets),
			//SecurityGroupIds: pulumi.StringArray{cluster.ClusterSecurityGroupId},
			PrivateDnsEnabled: pulumi.Bool(true),
		})
		
		if err != nil {
			return err
		}
		ctx.Export("ecrEndpointUrl", ecrEndpoint.Arn)
		

		dkrEndpoint, err := ec2.NewVpcEndpoint(ctx, ClusterName+"-"+"dkr-endpoint", &ec2.VpcEndpointArgs{
			VpcId:             vpc.ID(),
			ServiceName:       pulumi.String("com.amazonaws."+Region+".ecr.dkr"),
			VpcEndpointType:   pulumi.String("Interface"),
			SubnetIds:         pulumi.ToStringArrayOutput(privateSubnets),
			//SecurityGroupIds: pulumi.StringArray{cluster.ClusterSecurityGroupId},
			PrivateDnsEnabled: pulumi.Bool(true),
		})
		
		if err != nil {
			return err
		}
		ctx.Export("dkrEndpointUrl", dkrEndpoint.Arn)



		s3Endpoint, err := ec2.NewVpcEndpoint(ctx, ClusterName+"-"+"s3-endpoint", &ec2.VpcEndpointArgs{
			VpcId:             vpc.ID(),
			ServiceName:       pulumi.String("com.amazonaws."+Region+".s3"),
			VpcEndpointType:   pulumi.String("Gateway"),
			RouteTableIds: 	   pulumi.StringArray{pulumi.String("rtb-0ee3eb5e11c428c38")},
			//SecurityGroupIds: pulumi.StringArray{cluster.ClusterSecurityGroupId},
			Policy: pulumi.String(`{
                "Statement": [
                    {
                        "Sid": "Access-to-specific-bucket-only",
                        "Principal": "*",
                        "Action": ["s3:*"],
                        "Effect": "Allow",
                        "Resource": ["arn:aws:s3:::prod-` + Region + `-starport-layer-bucket/*"]
                    }
                ]
            }`),	
		})
		
		if err != nil {
			return err
		}
		ctx.Export("s3EndpointUrl", s3Endpoint.Arn)


		// Create an EKS cluster
		authMode := eks.AuthenticationModeApi 

		cluster, err := eks.NewCluster(ctx, ClusterName, &eks.ClusterArgs{
		
			NodeAssociatePublicIpAddress: pulumi.BoolRef(false),
			EndpointPrivateAccess: pulumi.Bool(true),
			EndpointPublicAccess: pulumi.Bool(true),
			AuthenticationMode: &authMode,
			/* We don't want expermiental AutoMode for use with Karpenter */
			// AutoMode: &eks.AutoModeOptionsArgs{
			// 	Enabled: true,
			// 	ComputeConfig: &eks.ClusterComputeConfigArgs{
			// 		NodeRoleArn: nodeRole.Arn,
			// 		NodePools: pulumi.ToStringArray([]string{"general-purpose"}),
			// 	},
			// },
			VpcId:           vpc.ID(),
			PrivateSubnetIds: pulumi.ToStringArrayOutput(privateSubnets),
			InstanceType:    pulumi.String(InstanceType),
			DesiredCapacity: pulumi.Int(2),
			MinSize:         pulumi.Int(1),
			MaxSize:         pulumi.Int(3),
			InstanceRole:    nodeRole, // Use the IAM role ARN
			SkipDefaultNodeGroup: pulumi.BoolRef(false),

		})
		if err != nil {
			return err
		}

		// Fix kubeconfig handling
        kubeconfig := cluster.Kubeconfig.ApplyT(func(k interface{}) (string, error) {
            kubeMap, ok := k.(map[string]interface{})
            if !ok {
                return "", fmt.Errorf("kubeconfig is not a map: %v", k)
            }
            // Use standard json.Marshal to get a string
            kubeBytes, err := json.Marshal(kubeMap)
            if err != nil {
                return "", fmt.Errorf("failed to marshal kubeconfig: %v", err)
            }
            return string(kubeBytes), nil
        }).(pulumi.StringOutput)

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{
            Kubeconfig: kubeconfig,
        })
        if err != nil {
            return err
        }

		ctx.Export("kubeconfig", kubeconfig)



		

		// Launch Kubernetes Deploys, Svc and Ingress
		fmt.Println("The K8s Provider:", k8sProvider.ToProviderOutput().OutputState)
		//start_deployments(ctx, k8sProvider)

		return nil
	})
}
