#!/bin/bash
GOOS=linux go build -o lambda_handler  main.go
upx lambda_handler
zip gamerelease_deployment.zip ./lambda_handler
aws lambda update-function-code \
  --region eu-west-1 \
  --function-name alexaHandler \
  --zip-file fileb://gamerelease_deployment.zip

