package client_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/krajorama/weaveworks-common-testclient/client"
	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/common/user"
)

func TestServerFunctions(t *testing.T) {
	serverUrl := os.Getenv("SERVER_URL")
	assert.NotEmpty(t, serverUrl, "Set the SERVER_URL environment variable")

	client, err := client.NewClient(serverUrl)

	assert.NoError(t, err, "Client creation")

	req, err := http.NewRequest("DELETE", "/end", &bytes.Buffer{})
	assert.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	// assert.Equal(t, "world", string(recorder.Body.Bytes()))
	assert.Equal(t, 200, recorder.Code)
}
