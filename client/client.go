package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/grpc-ecosystem/go-grpc-middleware"
	otgrpc "github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"github.com/weaveworks/common/httpgrpc"
	"github.com/weaveworks/common/logging"
	"github.com/weaveworks/common/middleware"
	"github.com/weaveworks/common/user"
	"google.golang.org/grpc"
)

// Client is a http.Handler that forwards the request over gRPC.
type Client struct {
	mtx       sync.RWMutex
	service   string
	namespace string
	port      string
	client    httpgrpc.HTTPClient
	conn      *grpc.ClientConn
}

func ParseURL(unparsed string) (string, error) {
	if strings.Contains(unparsed, ":///") {
		return unparsed, nil
	}

	parsed, err := url.Parse(unparsed)
	if err != nil {
		return "", err
	}

	scheme, host := parsed.Scheme, parsed.Host
	if !strings.Contains(unparsed, "://") {
		scheme, host = "direct", unparsed
	}

	switch scheme {
	case "direct":
		return host, err

	default:
		return "", fmt.Errorf("unrecognised scheme: %s", parsed.Scheme)
	}
}

// NewClient makes a new Client, given a kubernetes service address.
func NewClient(address string) (*Client, error) {
	// kuberesolver.RegisterInCluster()

	address, err := ParseURL(address)
	if err != nil {
		return nil, err
	}
	const grpcServiceConfig = `{"loadBalancingPolicy":"round_robin"}`

	dialOptions := []grpc.DialOption{
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(
			otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer()),
			middleware.ClientUserHeaderInterceptor,
		)),
	}

	conn, err := grpc.Dial(address, dialOptions...)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: httpgrpc.NewHTTPClient(conn),
		conn:   conn,
	}, nil
}

// HTTPRequest wraps an ordinary HTTPRequest with a gRPC one
func HTTPRequest(r *http.Request) (*httpgrpc.HTTPRequest, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return &httpgrpc.HTTPRequest{
		Method:  r.Method,
		Url:     r.URL.Path,
		Body:    body,
		Headers: fromHeader(r.Header),
	}, nil
}

// WriteResponse converts an httpgrpc response to an HTTP one
func WriteResponse(w http.ResponseWriter, resp *httpgrpc.HTTPResponse) error {
	toHeader(resp.Headers, w.Header())
	w.WriteHeader(int(resp.Code))
	_, err := w.Write(resp.Body)
	return err
}

// WriteError converts an httpgrpc error to an HTTP one
func WriteError(w http.ResponseWriter, err error) {
	resp, ok := httpgrpc.HTTPResponseFromError(err)
	if ok {
		WriteResponse(w, resp)
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ServeHTTP implements http.Handler
func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if tracer := opentracing.GlobalTracer(); tracer != nil {
		if span := opentracing.SpanFromContext(r.Context()); span != nil {
			if err := tracer.Inject(span.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header)); err != nil {
				logging.Global().Warnf("Failed to inject tracing headers into request: %v", err)
			}
		}
	}

	err := user.InjectOrgIDIntoHTTPRequest(r.Context(), r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req, err := HTTPRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := c.client.Handle(r.Context(), req)
	if err != nil {
		// Some errors will actually contain a valid resp, just need to unpack it
		var ok bool
		resp, ok = httpgrpc.HTTPResponseFromError(err)

		if !ok {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := WriteResponse(w, resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func toHeader(hs []*httpgrpc.Header, header http.Header) {
	for _, h := range hs {
		header[h.Key] = h.Values
	}
}

func fromHeader(hs http.Header) []*httpgrpc.Header {
	result := make([]*httpgrpc.Header, 0, len(hs))
	for k, vs := range hs {
		result = append(result, &httpgrpc.Header{
			Key:    k,
			Values: vs,
		})
	}
	return result
}
