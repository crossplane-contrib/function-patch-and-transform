package main

import (
	"net"
	"path/filepath"
	"time"

	"github.com/alecthomas/kong"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/crossplane/crossplane-runtime/pkg/certificates"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
)

// TODO(negz): Move all of this to github.com/crossplane/function-sdk-go.

// CLI of this Function.
type CLI struct {
	Debug bool `short:"d" help:"Emit debug logs in addition to info logs."`

	Network     string `help:"Network on which to listen for gRPC connections." default:"tcp"`
	Address     string `help:"Address at which to listen for gRPC connections." default:":9443"`
	TLSCertsDir string `help:"Folder containing server certs (tls.key, tls.crt) and the CA used to verify client certificates (ca.crt)" env:"TLS_SERVER_CERTS_DIR"`
	Insecure    bool   `help:"Run without mTLS credentials. If you supply this flag --tls-server-certs-dir will be ignored."`
}

// Run this Function.
func Run(description string) {
	ctx := kong.Parse(&CLI{}, kong.Description(description))
	ctx.FatalIfErrorf(ctx.Run())
}

// Run this Function's gRPC server.
func (c *CLI) Run() error {
	log, err := NewLogger(c.Debug)
	if err != nil {
		return errors.Wrap(err, "cannot create logger")
	}

	lis, err := net.Listen(c.Network, c.Address)
	if err != nil {
		return errors.Wrapf(err, "cannot listen for %s connections at address %q", c.Network, c.Address)
	}

	// TODO(negz): We'll need to inject the &Function{} implementation.

	if c.Insecure {
		srv := NewInsecureServer()
		fnv1beta1.RegisterFunctionRunnerServiceServer(srv, &Function{log: log})
		return errors.Wrap(srv.Serve(lis), "cannot serve gRPC connections")
	}

	srv, err := NewServer(c.TLSCertsDir)
	if err != nil {
		return errors.Wrap(err, "cannot create gRPC server")
	}
	fnv1beta1.RegisterFunctionRunnerServiceServer(srv, &Function{log: log})
	return errors.Wrap(srv.Serve(lis), "cannot serve gRPC connections")

}

// NewInsecureServer returns a gRPC server with no transport security.
func NewInsecureServer() *grpc.Server {
	srv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	reflection.Register(srv)
	return srv
}

// NewServer returns a gRPC server with mTLS transport security.
// It expects certsDir to contain valid server certs (tls.crt and tls.key), and
// the CA used to verify client certificates (ca.crt).
func NewServer(certsDir string) (*grpc.Server, error) {
	tlscfg, err := certificates.LoadMTLSConfig(
		filepath.Join(certsDir, "ca.crt"),
		filepath.Join(certsDir, "tls.crt"),
		filepath.Join(certsDir, "tls.key"),
		true)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot load mTLS configuration from %s", certsDir)
	}

	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlscfg)))
	reflection.Register(srv)
	return srv, nil
}

// NewLogger returns a new logger.
// TODO(negz): Use slog from the stdlib instead? Would require Go 1.21.
func NewLogger(debug bool) (logging.Logger, error) {
	o := []zap.Option{zap.AddCallerSkip(1)}
	if debug {
		o = append(o, zap.IncreaseLevel(zap.DebugLevel))
	}
	zl, err := zap.NewProduction(o...)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create zap logger")
	}
	return logging.NewLogrLogger(zapr.NewLogger(zl)), nil
}

// GetObject the supplied Kubernetes object from the supplied protobuf struct.
func GetObject(o runtime.Object, from *structpb.Struct) error {
	b, err := protojson.Marshal(from)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal %T to JSON", from)
	}

	if err := json.Unmarshal(b, o); err != nil {
		return errors.Wrapf(err, "cannot unmarshal JSON from %T into %T", from, o)
	}

	return nil
}

// NewResponseTo bootstraps a response to the supplied request. It automatically
// copies the desired state from the request.
func NewResponseTo(req *fnv1beta1.RunFunctionRequest, ttl time.Duration) *fnv1beta1.RunFunctionResponse {
	return &fnv1beta1.RunFunctionResponse{
		Meta: &fnv1beta1.ResponseMeta{
			Tag: req.GetMeta().GetTag(),
			Ttl: durationpb.New(ttl),
		},
		Desired: req.Desired,
	}
}

// Fatal adds a fatal result to the supplied RunFunctionResponse.
func Fatal(rsp *fnv1beta1.RunFunctionResponse, err error) {
	if rsp.Results == nil {
		rsp.Results = make([]*fnv1beta1.Result, 0, 1)
	}
	rsp.Results = append(rsp.Results, &fnv1beta1.Result{
		Severity: fnv1beta1.Severity_SEVERITY_FATAL,
		Message:  err.Error(),
	})
}

// GetObservedComposite resource from the supplied request.
func GetObservedComposite(req *fnv1beta1.RunFunctionRequest) (*composite.Unstructured, error) {
	cd := composite.New()
	err := GetObject(cd, req.GetObserved().GetComposite().GetResource())
	return cd, err
}

// GetDesiredComposite resource from the supplied request.
func GetDesiredComposite(req *fnv1beta1.RunFunctionRequest) (*composite.Unstructured, error) {
	cd := composite.New()
	err := GetObject(cd, req.GetDesired().GetComposite().GetResource())
	return cd, err
}
