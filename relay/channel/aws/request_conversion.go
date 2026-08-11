package aws

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel/awsbedrock"
	"github.com/gin-gonic/gin"
)

const awsBedrockAnthropicVersion = awsbedrock.AnthropicVersion

func convertAwsBedrockRequestBody(c *gin.Context, body []byte, header http.Header, modelNames ...string) ([]byte, bool, error) {
	return awsbedrock.ConvertRequestBodyWithOptions(c, body, header, awsbedrock.RequestConversionOptions{
		BetaModelNames: modelNames,
	})
}

func convertAwsBedrockRequestBodyWithURLLoader(c *gin.Context, body []byte, header http.Header, urlLoader awsbedrock.URLLoader) ([]byte, bool, error) {
	return awsbedrock.ConvertRequestBodyWithURLLoader(c, body, header, urlLoader)
}
