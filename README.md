# Building a Modern Application on AWS EKS with Pulumi: A Practical Journey

The application leverages PostgreSQL for data persistence, Kafka for event streaming, and a mix of Go, Python, and JavaScript components, all deployed on AWS Elastic Kubernetes Service (EKS) using Pulumi in Go. The cluster runs in private subnets with VPC endpoints for a fully isolated setup.

## The Go Code: Precision in Pulumi

The backbone of our infrastructure is a meticulously structured Go program using the Pulumi AWS and EKS SDKs (`github.com/pulumi/pulumi-aws/sdk/v6/go/aws` and `github.com/pulumi/pulumi-eks/sdk/v1/go/eks`). We defined a VPC with private subnets using `aws.ec2.Vpc` and `aws.ec2.Subnet`, ensuring each subnet has a dedicated route table created via `aws.ec2.RouteTable` and associated with `aws.ec2.RouteTableAssociation`. This setup isolates traffic, with `privateSubnets` passed as a `pulumi.StringArrayOutput` to the EKS cluster for node placement.

The EKS cluster itself is instantiated with `eks.NewCluster`, taking arguments like `VpcId`, `SubnetIds`, and `InstanceType` (e.g., `t3.medium`) to configure a default node group. A critical piece is the `InstanceRole`, an `aws.iam.Role` with a trust policy for `ec2.amazonaws.com`, attached to policies like `AmazonEKSWorkerNodePolicy`, `AmazonEKSVPCResourceController`, and `AmazonEKS_CNI_Policy` via `iam.NewRolePolicyAttachment`. This role, assigned to nodes, ensures proper cluster integration and CNI functionality. We set `EndpointPrivateAccess: true` and `EndpointPublicAccess: true` for flexibility, though nodes rely on private VPC endpoints.

To support private networking, we added VPC endpoints using `aws.ec2.VpcEndpoint`. The EKS endpoint (`com.amazonaws.us-east-1.eks`) and ECR endpoints (`com.amazonaws.us-east-1.ecr.dkr` and `.ecr.api`) are Interface types with `PrivateDnsEnabled: true`, tied to `privateSubnets` and the cluster’s node security group (`cluster.NodeSecurityGroupIds`). The S3 Gateway endpoint (`com.amazonaws.us-east-1.s3`) uses `RouteTableIds` derived dynamically from `privateSubnets` with an `ApplyT` function calling `ec2.LookupRouteTable`, corrected to target `arn:aws:s3:::prod-us-east-1-starport-layer-bucket/*` after fixing a `ctx.Stack()` bug. The final touch exports the kubeconfig (`ctx.Export("kubeconfig", cluster.Kubeconfig)`), enabling `kubectl` access.

## Challenge 1: Bootstrapping AL2023 Nodes

**Problem**: Nodes wouldn’t register; kubelet failed with “Failed to load environment files” errors, and `bootstrap.sh` was missing.

**Solution**: The AL2023 EKS AMI uses `nodeadm`. A security group blocked HTTPS to the EKS VPC endpoint (`com.amazonaws.us-east-1.eks`). We fixed the security group, confirmed connectivity with `curl`, and ran `/opt/eks/nodeadm join --config /etc/eks/nodeadm/config.yaml`. In Pulumi, we set `InstanceRole` in `eks.ClusterArgs`.

## Challenge 2: IAM Role Confusion

**Problem**: Nodes hit “Unauthorized” errors due to a misconfigured IAM role.

**Solution**: We used `ServiceRole` instead of `InstanceRole`. Switching to `nodeRole` with `AmazonEKSWorkerNodePolicy`, `AmazonEKSVPCResourceController`, and `AmazonEKS_CNI_Policy` fixed it. We mapped it in `aws-auth` ConfigMap (`system:nodes`) and added an `eks.AccessEntry` for `API_AND_CONFIG_MAP` mode, then ran `pulumi up`.

## Challenge 3: NetworkPluginNotReady

**Problem**: Nodes stayed “Not Ready” with `NetworkReady=false reason:NetworkPluginNotReady`.

**Solution**: The `aws-node` pod couldn’t pull its image from ECR without a VPC endpoint for `com.amazonaws.us-east-1.ecr.dkr`. We added the endpoint with `PrivateDnsEnabled: true`, restarted containerd and kubelet, and deleted the pod to retry.

## Challenge 4: S3 Access Denied

**Problem**: A 403 Forbidden error occurred when fetching CNI image blobs from S3 (`prod-us-east-1-starport-layer-bucket`), despite ECR working.

**Solution**: We added an S3 Gateway endpoint (`com.amazonaws.us-east-1.s3`) with a policy for `arn:aws:s3:::prod-us-east-1-starport-layer-bucket/*`. A Pulumi bug—`ctx.Stack()` returning `prod` instead of `us-east-1`—misnamed the bucket as `prod-prod-`. Fixing it to `us-east-1` aligned the policy, resolving the 403.

## Challenge 5: Containerd vs. Docker

**Problem**: Debugging assumed Docker, but AL2023 uses containerd—no `docker` command.

**Solution**: We used `ctr` for testing (`sudo ctr image pull ...`) and confirmed containerd’s config (`/etc/containerd/config.toml`) relied on IMDS for ECR creds.

## The Outcome

After `pulumi up`, nodes registered and flipped to “Ready” (`kubectl get nodes`), and `aws-node` pods ran (`kubectl get pods -n kube-system`), initializing the CNI.

## Key Takeaways

- **AL2023 Shift**: Use `nodeadm` and `InstanceRole` for node IAM.
- **Private Networking**: Add VPC endpoints (EKS, ECR, S3) and test with `curl`.
- **IAM Precision**: Verify roles and stack names.
- **S3 Nuance**: Fix ECR-to-S3 redirects with correct bucket policies.