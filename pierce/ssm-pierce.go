package pierce

import (
	"context"
	"fmt"
	"github.com/Cyberax/gimlet"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"math/rand"
)

// PierceVpcVeil prepares a tunnel to the `targetHost:port` endpoint via the EC2 instance identified by
// `instanceId`. Leave `targetHost` empty to connect to the EC2 instance identified by `instanceId` itself.
// Returns the ConnInfo object that can be serialized into JSON and communicated to another host that will
// in turn use it to open the Gimlet channel using gimlet.NewChannel call.
func PierceVpcVeil(ctx context.Context, config aws.Config,
	instanceId string, targetHost string, port uint16) (*gimlet.ConnInfo, error) {

	cli := ssm.NewFromConfig(config)

	var session *ssm.StartSessionOutput

	if targetHost != "" {
		params := map[string][]string{
			"host":            {targetHost},
			"portNumber":      {fmt.Sprintf("%d", port)},
			"localPortNumber": {fmt.Sprintf("%d", rand.Intn(65534)+1)},
		}

		var err error
		session, err = cli.StartSession(ctx, &ssm.StartSessionInput{
			Target:       aws.String(instanceId),
			DocumentName: aws.String("AWS-StartPortForwardingSessionToRemoteHost"),
			Parameters:   params,
		})
		if err != nil {
			return nil, err
		}
	} else {
		params := map[string][]string{
			"portNumber":      {fmt.Sprintf("%d", port)},
			"localPortNumber": {fmt.Sprintf("%d", rand.Intn(65534)+1)},
		}

		var err error
		session, err = cli.StartSession(ctx, &ssm.StartSessionInput{
			Target:       aws.String(instanceId),
			DocumentName: aws.String("AWS-StartPortForwardingSession"),
			Parameters:   params,
		})
		if err != nil {
			return nil, err
		}
	}

	return &gimlet.ConnInfo{
		InstanceId: instanceId,
		Region:     config.Region,
		Endpoint:   *session.StreamUrl,
		SessionId:  *session.SessionId,
		Token:      *session.TokenValue,
	}, nil
}
