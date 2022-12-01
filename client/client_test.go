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
	jaegercfg "github.com/uber/jaeger-client-go/config"
	opentracing "github.com/opentracing/opentracing-go"
)

func TestBasic(t *testing.T) {
	client := createClient(t)

	req, err := http.NewRequest("GET", "/hello", &bytes.Buffer{})
	assert.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "world", recorder.Body.String())
	assert.Equal(t, 200, recorder.Code)
}


func TestError500(t *testing.T) {
	client := createClient(t)

	req, err := http.NewRequest("GET", "/error500", &bytes.Buffer{})
	assert.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "server error message\n", recorder.Body.String())
	assert.Equal(t, 500, recorder.Code)
}

func TestError400(t *testing.T) {
	client := createClient(t)

	req, err := http.NewRequest("GET", "/error400", &bytes.Buffer{})
	assert.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "request error message\n", recorder.Body.String())
	assert.Equal(t, 403, recorder.Code)
}

func TestTracePropagation(t *testing.T) {
	jaeger := jaegercfg.Configuration{}
    closer, err := jaeger.InitGlobalTracer("test")
	assert.NoError(t, err)
	defer closer.Close()

	client := createClient(t)

	req, err := http.NewRequest("GET", "/trace", &bytes.Buffer{})
	assert.NoError(t, err)

	sp, ctx := opentracing.StartSpanFromContext(context.Background(), "Test")
	sp.SetBaggageItem("name", "tracedata")

	req = req.WithContext(user.InjectOrgID(ctx, "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "tracedata", recorder.Body.String())
	assert.Equal(t, 200, recorder.Code)
}


func TestOrgPassed(t *testing.T) {
	client := createClient(t)

	req, err := http.NewRequest("GET", "/orgid", &bytes.Buffer{})
	assert.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "anonymous"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, "anonymous", recorder.Body.String())
	assert.Equal(t, 200, recorder.Code)
}

func TestServerShutdown(t *testing.T) {
	client := createClient(t)

	req, err := http.NewRequest("DELETE", "/end", &bytes.Buffer{})
	assert.NoError(t, err)

	req = req.WithContext(user.InjectOrgID(context.Background(), "1"))
	recorder := httptest.NewRecorder()
	client.ServeHTTP(recorder, req)

	assert.Equal(t, 200, recorder.Code)
}

func createClient(t *testing.T) *client.Client {
	serverUrl := os.Getenv("SERVER_URL")
	assert.NotEmpty(t, serverUrl, "Set the SERVER_URL environment variable")

	client, err := client.NewClient(serverUrl)

	assert.NoError(t, err, "Client creation")
	return client
}
