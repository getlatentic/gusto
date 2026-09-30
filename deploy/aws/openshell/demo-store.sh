#!/usr/bin/env bash
# Makes the bucket that holds what should outlive a demo host: the wildcard certificate,
# which Let's Encrypt limits to five a week, and the runner image, which takes ten minutes
# to build. Making it again finds it. It costs a few cents a month, ./down.sh leaves it,
# and it prints its name.
source "$(dirname "$0")/env.sh"
account="$(aws sts get-caller-identity --query Account --output text)"
bucket="haas-poc-demo-$account"
if ! aws s3api head-bucket --bucket "$bucket" 2>/dev/null; then
  if [ "$AWS_REGION" = us-east-1 ]; then
    aws s3api create-bucket --bucket "$bucket" >/dev/null
  else
    aws s3api create-bucket --bucket "$bucket" --create-bucket-configuration "LocationConstraint=$AWS_REGION" >/dev/null
  fi
fi
aws s3api put-public-access-block --bucket "$bucket" \
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
echo "$bucket"
