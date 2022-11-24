package gcr

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"golang.org/x/oauth2"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/random"
)

// Helper functions below (newTLSServer, etc.) have been copied from https://github.com/google/go-containerregistry

type fakeRepo struct {
	h     http.Handler
	repos map[string]google.Tags
}

func (fr *fakeRepo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	glog.Infof("%s %s", r.Method, r.URL)
	if strings.HasPrefix(r.URL.Path, "/v2/") && strings.HasSuffix(r.URL.Path, "/tags/list") {
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/"), "/tags/list")
		if tags, ok := fr.repos[repo]; !ok {
			w.WriteHeader(http.StatusNotFound)
		} else {
			glog.Infof("%+v", tags)
			if err := json.NewEncoder(w).Encode(tags); err != nil {
				glog.Exit(err)
			}
		}
	} else {
		fr.h.ServeHTTP(w, r)
	}
}

func newFakeRepo(stuff map[name.Reference]partial.Describable) (*fakeRepo, error) {
	h := registry.New()
	repos := make(map[string]google.Tags)

	for ref, thing := range stuff {
		repo := ref.Context().RepositoryStr()
		tags, ok := repos[repo]
		if !ok {
			tags = google.Tags{
				Name:     repo,
				Children: []string{},
			}
		}

		// Populate the "child" field.
		for parentPath := repo; parentPath != "."; parentPath = path.Dir(parentPath) {
			child, parent := path.Base(parentPath), path.Dir(parentPath)
			tags, ok := repos[parent]
			if !ok {
				tags = google.Tags{}
			}
			for _, c := range repos[parent].Children {
				if c == child {
					break
				}
			}
			tags.Children = append(tags.Children, child)
			repos[parent] = tags
		}

		// Populate the "manifests" and "tags" field.
		d, err := thing.Digest()
		if err != nil {
			return nil, err
		}
		mt, err := thing.MediaType()
		if err != nil {
			return nil, err
		}
		if tags.Manifests == nil {
			tags.Manifests = make(map[string]google.ManifestInfo)
		}
		mi, ok := tags.Manifests[d.String()]
		if !ok {
			mi = google.ManifestInfo{
				MediaType: string(mt),
				Tags:      []string{},
			}
		}
		if tag, ok := ref.(name.Tag); ok {
			tags.Tags = append(tags.Tags, tag.Identifier())
			mi.Tags = append(mi.Tags, tag.Identifier())
		}
		tags.Manifests[d.String()] = mi
		repos[repo] = tags
	}

	return &fakeRepo{h: h, repos: repos}, nil
}

func getTestRepo() (*fakeRepo, []*image, error) {
	image1, err := random.Image(1024, 5)
	if err != nil {
		return nil, nil, err
	}

	ha1, err := image1.Digest()
	if err != nil {
		return nil, nil, err
	}
	fmt.Println(ha1.Hex)

	image1name := "registry.example.com/test/hashr/aaa"
	lr1, err := name.ParseReference(image1name)
	if err != nil {
		return nil, nil, err
	}

	ref1 := lr1.Context().Tag("foo")

	image2, err := random.Image(1024, 5)
	if err != nil {
		return nil, nil, err
	}

	ha2, err := image2.Digest()
	if err != nil {
		return nil, nil, err
	}
	fmt.Println(ha2.Hex)

	image2name := "registry.example.com/test/hashr/bbb"
	lr2, err := name.ParseReference(image2name)
	if err != nil {
		return nil, nil, err
	}

	ref2 := lr2.Context().Tag("bar")

	wantImages := []*image{
		{
			id:          image1name,
			quickHash:   ha1.Hex,
			description: "Tags: [foo], Media Type: application/vnd.docker.distribution.manifest.v2+json, Created on: 1754-08-30 23:17:49.129 +0034 LMT, Uploaded on: 1754-08-30 23:17:49.129 +0034 LMT",
			remotePath:  image1name,
		},
		{
			id:          image2name,
			quickHash:   ha2.Hex,
			description: "Tags: [bar], Media Type: application/vnd.docker.distribution.manifest.v2+json, Created on: 1754-08-30 23:17:49.129 +0034 LMT, Uploaded on: 1754-08-30 23:17:49.129 +0034 LMT",
			remotePath:  image2name,
		},
	}

	// Set up a fake registry.
	h, err := newFakeRepo(map[name.Reference]partial.Describable{
		ref1: image1,
		ref2: image2,
	})
	if err != nil {
		return nil, nil, err
	}

	return h, wantImages, nil
}

func TestDiscoverRepo(t *testing.T) {
	fakeRepo, wantImages, err := getTestRepo()
	if err != nil {
		t.Fatalf("could not create fake GCR repo: %v", err)
	}

	s, err := newTLSServer("registry.example.com", fakeRepo)
	if err != nil {
		glog.Exit(err)
	}
	defer s.Close()

	repo, err := NewRepo(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{}), "registry.example.com/test/hashr")
	if err != nil {
		t.Fatalf("could not create new GCR repo: %v", err)
	}

	// Route requests to our test registry.
	opts = google.WithTransport(s.Client().Transport)

	gotSources, err := repo.DiscoverRepo()
	if err != nil {
		t.Fatalf("unexpected error in DiscoverRepo(): %v", err)
	}

	var gotImages []*image
	for _, source := range gotSources {
		if image, ok := source.(*image); ok {
			gotImages = append(gotImages, image)
		} else {
			t.Fatal("error while casting Source interface to Image struct")
		}
	}

	if !cmp.Equal(wantImages, gotImages, cmp.AllowUnexported(image{})) {
		t.Errorf("DiscoverRepo() unexpected diff (-want/+got):\n%s", cmp.Diff(wantImages, gotImages, cmp.AllowUnexported(image{})))
	}
}

func newTLSServer(domain string, handler http.Handler) (*httptest.Server, error) {
	s := httptest.NewUnstartedServer(handler)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses: []net.IP{
			net.IPv4(127, 0, 0, 1),
			net.IPv6loopback,
		},
		DNSNames: []string{domain},

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, err
	}

	b, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	pc := &bytes.Buffer{}
	if err := pem.Encode(pc, &pem.Block{Type: "CERTIFICATE", Bytes: b}); err != nil {
		return nil, err
	}

	ek, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	pk := &bytes.Buffer{}
	if err := pem.Encode(pk, &pem.Block{Type: "EC PRIVATE KEY", Bytes: ek}); err != nil {
		return nil, err
	}

	c, err := tls.X509KeyPair(pc.Bytes(), pk.Bytes())
	if err != nil {
		return nil, err
	}
	s.TLS = &tls.Config{
		Certificates: []tls.Certificate{c},
	}
	s.StartTLS()

	certpool := x509.NewCertPool()
	certpool.AddCert(s.Certificate())

	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certpool,
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial(s.Listener.Addr().Network(), s.Listener.Addr().String())
		},
	}
	s.Client().Transport = t

	return s, nil
}
