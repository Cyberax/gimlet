module github.com/Cyberax/gimlet/gimlet-tool

go 1.18

require github.com/aws/aws-sdk-go-v2/config v1.18.5

require (
	github.com/Cyberax/gimlet v1.0.2
	github.com/Cyberax/gimlet/pierce v1.0.2
)

require (
	github.com/aws/aws-sdk-go-v2 v1.17.3 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.5 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.11.27 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.13.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.17.7 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/xtaci/smux v1.5.17 // indirect
	golang.org/x/net v0.4.0 // indirect
)

replace github.com/Cyberax/gimlet => ../

replace github.com/Cyberax/gimlet/pierce => ../pierce
