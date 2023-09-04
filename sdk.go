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
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composed"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"

	"github.com/crossplane/function-sdk-go/proto/v1beta1"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
)

// TODO(negz): Move all of this to github.com/crossplane/function-sdk-go.

// DefaultTTL is the default TTL for which a Function response can be cached.
const DefaultTTL = 1 * time.Minute

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

// TODO(negz): Less dumb API for NewServer.
// TODO(negz): Inject (and configure) gRPC otel interceptor.
// TODO(negz): Do we need to handle cancelled contexts and deadlines ourself?

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

// GetObject gets the supplied Kubernetes object from the supplied struct.
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

// GetStruct gets the supplied struct from the supplied Kubernetes object.
func GetStruct(s *structpb.Struct, from runtime.Object) error {
	b, err := json.Marshal(from)
	if err != nil {
		return errors.Wrapf(err, "cannot marshal %T to JSON", from)
	}

	if err := protojson.Unmarshal(b, s); err != nil {
		return errors.Wrapf(err, "cannot unmarshal JSON from %T into %T", from, s)
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

// TODO(negz): Inject the step name into the RunFunctionRequest so folks can
// tell which Function returned which results.

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

// Warning adds a warning result to the supplied RunFunctionResponse.
func Warning(rsp *fnv1beta1.RunFunctionResponse, err error) {
	if rsp.Results == nil {
		rsp.Results = make([]*fnv1beta1.Result, 0, 1)
	}
	rsp.Results = append(rsp.Results, &fnv1beta1.Result{
		Severity: fnv1beta1.Severity_SEVERITY_WARNING,
		Message:  err.Error(),
	})
}

// A CompositeResource - aka an XR.
type CompositeResource struct {
	Resource          *composite.Unstructured
	ConnectionDetails managed.ConnectionDetails
}

// GetObservedCompositeResource from the supplied request.
func GetObservedCompositeResource(req *fnv1beta1.RunFunctionRequest) (*CompositeResource, error) {
	xr := &CompositeResource{
		Resource:          composite.New(),
		ConnectionDetails: req.GetObserved().GetComposite().GetConnectionDetails(),
	}
	err := GetObject(xr.Resource, req.GetObserved().GetComposite().GetResource())
	return xr, err
}

// GetDesiredCompositeResource from the supplied request.
func GetDesiredCompositeResource(req *fnv1beta1.RunFunctionRequest) (*CompositeResource, error) {
	xr := &CompositeResource{
		Resource:          composite.New(),
		ConnectionDetails: req.GetDesired().GetComposite().GetConnectionDetails(),
	}
	err := GetObject(xr.Resource, req.GetDesired().GetComposite().GetResource())
	return xr, err
}

// A ComposedResourceName uniquely identifies a composed resource within a
// Composition Function pipeline. It's not the resource's metadata.name.
type ComposedResourceName string

// An ObservedComposedResource is the observed state of a composed resource.
type ObservedComposedResource struct {
	Resource          *composed.Unstructured
	ConnectionDetails managed.ConnectionDetails
}

// ObservedComposedResources indexed by resource name.
type ObservedComposedResources map[ComposedResourceName]ObservedComposedResource

// GetObservedComposedResources from the supplied request.
func GetObservedComposedResources(req *fnv1beta1.RunFunctionRequest) (ObservedComposedResources, error) {
	ocds := ObservedComposedResources{}
	for name, r := range req.GetObserved().GetResources() {
		ocd := ObservedComposedResource{Resource: composed.New(), ConnectionDetails: r.GetConnectionDetails()}
		if err := GetObject(ocd.Resource, r.GetResource()); err != nil {
			return nil, err
		}
		ocds[ComposedResourceName(name)] = ocd
	}
	return ocds, nil
}

// A DesiredComposedResource is the desired state of a composed resource.
type DesiredComposedResource struct {
	Resource *composed.Unstructured

	// TODO(negz): Ternary enum for readiness - unknown, true, false? Is this
	// actually something we'd return in the desired resource, or observed?
	// We're observing that the resource is ready, not desiring it to be ready.
}

// DesiredComposedResources  indexed by resource name.
type DesiredComposedResources map[ComposedResourceName]DesiredComposedResource

// GetDesiredComposedResources from the supplied request.
func GetDesiredComposedResources(req *fnv1beta1.RunFunctionRequest) (DesiredComposedResources, error) {
	ocds := DesiredComposedResources{}
	for name, r := range req.GetDesired().GetResources() {
		ocd := DesiredComposedResource{Resource: composed.New()}
		if err := GetObject(ocd.Resource, r.GetResource()); err != nil {
			return nil, err
		}
		ocds[ComposedResourceName(name)] = ocd
	}
	return ocds, nil
}

// NewDesiredComposedResource returns a new, empty desired composed resource.
func NewDesiredComposedResource() DesiredComposedResource {
	return DesiredComposedResource{Resource: composed.New()}
}

// SetDesiredCompositeResource sets the desired composite resource in the
// supplied response. The caller must be sure to avoid overwriting the desired
// state that may have been accumulated by previous Functions in the pipeline,
// unless they intend to.
func SetDesiredCompositeResource(rsp *v1beta1.RunFunctionResponse, xr *CompositeResource) error {
	if rsp.Desired == nil {
		rsp.Desired = &v1beta1.State{}
	}
	rsp.Desired.Composite = &v1beta1.Resource{
		Resource:          &structpb.Struct{},
		ConnectionDetails: xr.ConnectionDetails,
	}
	return GetStruct(rsp.Desired.Composite.Resource, xr.Resource)
}

// SetDesiredComposedResources sets the desired composed resources in the
// supplied response. The caller must be sure to avoid overwriting the desired
// state that may have been accumulated by previous Functions in the pipeline,
// unless they intend to.
func SetDesiredComposedResources(rsp *v1beta1.RunFunctionResponse, dcds DesiredComposedResources) error {
	if rsp.Desired == nil {
		rsp.Desired = &v1beta1.State{}
	}
	if rsp.Desired.Resources == nil {
		rsp.Desired.Resources = map[string]*fnv1beta1.Resource{}
	}
	for name, dcd := range dcds {
		r := &v1beta1.Resource{Resource: &structpb.Struct{}}
		if err := GetStruct(r.Resource, dcd.Resource); err != nil {
			return err
		}
		rsp.Desired.Resources[string(name)] = r
	}
	return nil
}

// TODO(negz): Stuff below this line is intended for testing.

// MustStructObject returns the supplied object as a struct.
// It panics if it can't.
func MustStructObject(o runtime.Object) *structpb.Struct {
	s := &structpb.Struct{}
	if err := GetStruct(s, o); err != nil {
		panic(err)
	}
	return s
}

// MustStructJSON returns the supplied JSON string as a struct.
// It panics if it can't.
func MustStructJSON(j string) *structpb.Struct {
	s := &structpb.Struct{}
	if err := protojson.Unmarshal([]byte(j), s); err != nil {
		panic(err)
	}
	return s
}
