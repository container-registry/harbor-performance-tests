# Jenkins pipeline to run performance test

## Prerequisites

1. Install a jenkins server
1. Configure the following environment variables in jenkins global configuration
    - CNCF_EKS_ACCESS_KEY with the username(access key) and password (secret key), the user must have permission to create/delete eks clusters
1. Add a jenkins node to the jenkins server, add label `aws-ec2` to the node.
1. Create the following three jenkins jobs
    - create-cluster  -- create/Jenkinsfile
    - run-performance-test  -- test/Jenkinsfile
    - delete-cluster  -- delete/Jenkinsfile
