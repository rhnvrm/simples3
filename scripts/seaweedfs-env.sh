# Public credentials for the local SeaweedFS test fixture; never use in production.
export AWS_S3_BUCKET=testbucket
export AWS_S3_ENDPOINT=http://127.0.0.1:9000
export AWS_S3_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true
export AWS_S3_ACCESS_KEY=simples3-test
export AWS_S3_SECRET_KEY=simples3-test-secret
export AWS_ACCESS_KEY_ID="$AWS_S3_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$AWS_S3_SECRET_KEY"
export AWS_DEFAULT_REGION="$AWS_S3_REGION"
