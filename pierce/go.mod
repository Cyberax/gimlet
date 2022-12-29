module github.com/Cyberax/gimlet/pierce

go 1.18

require github.com/aws/aws-sdk-go-v2 v1.17.3

require (
	github.com/Cyberax/gimlet v1.0.2
	github.com/aws/aws-sdk-go-v2/config v1.18.5
	github.com/stretchr/testify v1.7.0
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.13.5 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.28 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.11.27 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.13.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.17.7 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/xtaci/smux v1.5.17 // indirect
	golang.org/x/net v0.4.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

require (
	github.com/aws/aws-sdk-go-v2/service/ssm v1.3.0
	github.com/aws/smithy-go v1.13.5 // indirect
)

replace github.com/Cyberax/gimlet => ../
