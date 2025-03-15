package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-eks/sdk/v2/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	name := "gore"
	clusterName := name+"-100"
	vpcName := name+"-100"
	pulumi.Run(func(ctx *pulumi.Context) error {
		// build a custom VPC
		vpc, _ := aws.ec2.NewVpc(ctx, vpcName, &aws.ec2.VpcArgs{
			CidrBlock: pulumi.String("172.27.0.0/16"),
		})

		// Create IAM role for node group
        nodeRole, err := CreateNodeRole(ctx)
        if err != nil {
            return err
        }
		// Create an EKS cluster
		cluster, err := eks.NewCluster(ctx, clusterName, &eks.ClusterArgs{
			VpcId:           vpc.ID(),
			InstanceType:    pulumi.String("t3.medium"),
			DesiredCapacity: pulumi.Int(2),
			MinSize:         pulumi.Int(1),
			MaxSize:         pulumi.Int(3),
			NodeRoleArn:     nodeRole.Arn, // Use the IAM role ARN
		})
		if err != nil {
			return err
		}

		// Kubernetes provider using the cluster's kubeconfig
		kubeconfig := cluster.Kubeconfig.ApplyT(func(k interface{}) string {
			return k.(string)
		}).(pulumi.StringOutput)
		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{
			Kubeconfig: kubeconfig,
		})
		if err != nil {
			return err
		}

		// App labels for selector
		appLabels := pulumi.StringMap{
			"app": pulumi.String("py-go-app"),
		}

		// Python Deployment
		_, err = appsv1.NewDeployment(ctx, "python-app", &appsv1.DeploymentArgs{
			Spec: appsv1.DeploymentSpecArgs{
				Selector: &corev1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{"app": pulumi.String("python")},
				},
				Replicas: pulumi.Int(1),
				Template: corev1.PodTemplateSpecArgs{
					Metadata: &corev1.ObjectMetaArgs{
						Labels: pulumi.StringMap{"app": pulumi.String("python")},
					},
					Spec: corev1.PodSpecArgs{
						Containers: corev1.ContainerArray{
							corev1.ContainerArgs{
								Name:  pulumi.String("fastapi"),
								Image: pulumi.String("your-registry/python-app:latest"),
								Ports: corev1.ContainerPortArray{
									corev1.ContainerPortArgs{
										ContainerPort: pulumi.Int(8000),
									},
								},
							},
						},
					},
				},
			},
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		// Go Deployment
		_, err = appsv1.NewDeployment(ctx, "go-app", &appsv1.DeploymentArgs{
			Spec: appsv1.DeploymentSpecArgs{
				Selector: &corev1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{"app": pulumi.String("go")},
				},
				Replicas: pulumi.Int(1),
				Template: corev1.PodTemplateSpecArgs{
					Metadata: &corev1.ObjectMetaArgs{
						Labels: pulumi.StringMap{"app": pulumi.String("go")},
					},
					Spec: corev1.PodSpecArgs{
						Containers: corev1.ContainerArray{
							corev1.ContainerArgs{
								Name:  pulumi.String("gorilla"),
								Image: pulumi.String("your-registry/go-app:latest"),
								Ports: corev1.ContainerPortArray{
									corev1.ContainerPortArgs{
										ContainerPort: pulumi.Int(8080),
									},
								},
							},
						},
					},
				},
			},
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		// Load Balancer Service
		lb, err := corev1.NewService(ctx, "py-go-lb", &corev1.ServiceArgs{
			Metadata: &corev1.ObjectMetaArgs{
				Annotations: pulumi.StringMap{
					"service.beta.kubernetes.io/aws-load-balancer-type": pulumi.String("alb"),
				},
			},
			Spec: corev1.ServiceSpecArgs{
				Type: pulumi.String("LoadBalancer"),
				Ports: corev1.ServicePortArray{
					corev1.ServicePortArgs{
						Port:       pulumi.Int(80),
						TargetPort: pulumi.Int(8000),
						Name:       pulumi.String("python"),
					},
					corev1.ServicePortArgs{
						Port:       pulumi.Int(80),
						TargetPort: pulumi.Int(8080),
						Name:       pulumi.String("go"),
					},
				},
				Selector: appLabels,
			},
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		// Ingress for path-based routing
		_, err = networkingv1.NewIngress(ctx, "py-go-ingress", &networkingv1.IngressArgs{
			Metadata: &corev1.ObjectMetaArgs{
				Annotations: pulumi.StringMap{
					"kubernetes.io/ingress.class":      pulumi.String("alb"),
					"alb.ingress.kubernetes.io/scheme": pulumi.String("internet-facing"),
				},
			},
			Spec: networkingv1.IngressSpecArgs{
				Rules: networkingv1.IngressRuleArray{
					&networkingv1.IngressRuleArgs{
						Http: &networkingv1.HTTPIngressRuleValueArgs{
							Paths: networkingv1.HTTPIngressPathArray{
								&networkingv1.HTTPIngressPathArgs{
									Path:     pulumi.String("/python"),
									PathType: pulumi.String("Prefix"),
									Backend: networkingv1.IngressBackendArgs{
										Service: &networkingv1.IngressServiceBackendArgs{
											Name: pulumi.String("python-service"),
											Port: networkingv1.ServiceBackendPortArgs{
												Number: pulumi.Int(8000),
											},
										},
									},
								},
								&networkingv1.HTTPIngressPathArgs{
									Path:     pulumi.String("/go"),
									PathType: pulumi.String("Prefix"),
									Backend: networkingv1.IngressBackendArgs{
										Service: &networkingv1.IngressServiceBackendArgs{
											Name: pulumi.String("go-service"),
											Port: networkingv1.ServiceBackendPortArgs{
												Number: pulumi.Int(8080),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, pulumi.Provider(k8sProvider))
		if err != nil {
			return err
		}

		// Export the Load Balancer URL
		lbUrl := lb.Status.ApplyT(func(status *corev1.ServiceStatus) string {
			if status != nil && status.LoadBalancer != nil && len(status.LoadBalancer.Ingress) > 0 {
				return status.LoadBalancer.Ingress[0].Hostname
			}
			return "pending"
		}).(pulumi.StringOutput)
		ctx.Export("lbUrl", lbUrl)

		return nil
	})
}
