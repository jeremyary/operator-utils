package platform

import (
	"testing"

	openapi_v2 "github.com/googleapis/gnostic/OpenAPIv2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
)

type FakeDiscoverer struct {
	info               PlatformInfo
	serverInfo         *version.Info
	groupList          *v1.APIGroupList
	doc                *openapi_v2.Document
	client             rest.Interface
	ServerVersionError error
	ServerGroupsError  error
	OpenAPISchemaError error
}

func (d FakeDiscoverer) ServerVersion() (*version.Info, error) {
	if d.ServerVersionError != nil {
		return nil, d.ServerVersionError
	}
	return d.serverInfo, nil
}

func (d FakeDiscoverer) ServerGroups() (*v1.APIGroupList, error) {
	if d.ServerGroupsError != nil {
		return nil, d.ServerGroupsError
	}
	return d.groupList, nil
}

func (d FakeDiscoverer) OpenAPISchema() (*openapi_v2.Document, error) {
	if d.OpenAPISchemaError != nil {
		return nil, d.OpenAPISchemaError
	}
	return d.doc, nil
}

func (d FakeDiscoverer) RESTClient() rest.Interface {
	return d.client
}

type FakePlatformVersioner struct {
	Info PlatformInfo
	Err  error
}

func (pv FakePlatformVersioner) GetPlatformInfo(d Discoverer, cfg *rest.Config) (PlatformInfo, error) {
	if pv.Err != nil {
		return pv.Info, pv.Err
	}
	return pv.Info, nil
}

func TestDetectOpenShift(t *testing.T) {

	ocpInfo := PlatformInfo{
		Name:       OpenShift,
		OCPVersion: "1.2.3",
		K8SVersion: "4.5.6",
		OS:         "foo/bar",
	}
	k8sInfo := ocpInfo
	k8sInfo.Name = Kubernetes

	cases := []struct {
		pv           FakePlatformVersioner
		cfg          *rest.Config
		expectedBool bool
		expectedErr  error
	}{
		{
			pv: FakePlatformVersioner{
				Info: ocpInfo,
				Err:  nil,
			},
			cfg:          &rest.Config{},
			expectedBool: true,
			expectedErr:  nil,
		},
		{
			pv: FakePlatformVersioner{
				Info: k8sInfo,
				Err:  nil,
			},
			cfg:          &rest.Config{},
			expectedBool: false,
			expectedErr:  nil,
		},
		{
			pv: FakePlatformVersioner{
				Info: ocpInfo,
				Err:  errors.New("uh oh"),
			},
			cfg:          &rest.Config{},
			expectedBool: false,
			expectedErr:  ErrInfoFetch,
		},
	}

	for _, c := range cases {
		IsOpenShift, err := DetectOpenShift(c.pv, c.cfg)
		assert.Equal(t, c.expectedBool, IsOpenShift, "mismatch in returned boolean result")
		assert.Equal(t, c.expectedErr, err, "mismatch in returned error")
	}
}

func TestK8SBasedPlatformVersioner_GetPlatformInfo(t *testing.T) {

	pv := K8SBasedPlatformVersioner{}

	cases := []struct {
		discoverer   Discoverer
		config       *rest.Config
		expectedInfo PlatformInfo
		expectedErr  error
	}{
		// CASE 1
		// trigger error in client.ServerVersion(), only Name present on Info
		{
			discoverer: FakeDiscoverer{
				ServerVersionError: errors.New("oops"),
			},
			config:       &rest.Config{},
			expectedInfo: PlatformInfo{Name: Kubernetes},
			expectedErr:  ErrK8SVersionFetch,
		},
		// CASE 2
		// trigger error in client.ServerGroups(), K8S major/minor now present from ServerVersion() result
		{
			discoverer: FakeDiscoverer{
				ServerGroupsError: errors.New("oops"),
				serverInfo: &version.Info{
					Major: "1",
					Minor: "2",
				},
			},
			config:       &rest.Config{},
			expectedInfo: PlatformInfo{Name: Kubernetes, K8SVersion: "1.2"},
			expectedErr:  ErrServerGroupsFetch,
		},
		// CASE 3
		// trigger no errors, simulate K8S platform (no OCP route present)
		{
			discoverer: FakeDiscoverer{
				serverInfo: &version.Info{
					Major: "1",
					Minor: "2",
				},
				groupList: &v1.APIGroupList{
					TypeMeta: v1.TypeMeta{},
					Groups:   []v1.APIGroup{},
				},
			},
			config:       &rest.Config{},
			expectedInfo: PlatformInfo{Name: Kubernetes, K8SVersion: "1.2"},
			expectedErr:  nil,
		},
		// CASE 4
		// trigger error in OpenAPISchema, info should now be OCP with K8S major/minor
		{
			discoverer: FakeDiscoverer{
				OpenAPISchemaError: errors.New("oops"),
				serverInfo: &version.Info{
					Major: "1",
					Minor: "2",
				},
				groupList: &v1.APIGroupList{
					TypeMeta: v1.TypeMeta{},
					Groups: []v1.APIGroup{
						{
							Name: "route.openshift.io",
						},
					},
				},
			},
			config:       &rest.Config{},
			expectedInfo: PlatformInfo{Name: OpenShift, K8SVersion: "1.2"},
			expectedErr:  ErrOpenAPISchemaFetch,
		},
		// CASE 5
		// trigger no error, let OCP version start with "3.1", info should now reflect this
		{
			discoverer: FakeDiscoverer{
				serverInfo: &version.Info{
					Major: "1",
					Minor: "2",
				},
				groupList: &v1.APIGroupList{
					TypeMeta: v1.TypeMeta{},
					Groups: []v1.APIGroup{
						{
							Name: "route.openshift.io",
						},
					},
				},
				doc: &openapi_v2.Document{
					Info: &openapi_v2.Info{
						Version: "v3.11.42",
					},
				},
			},
			config:       &rest.Config{},
			expectedInfo: PlatformInfo{Name: OpenShift, K8SVersion: "1.2", OCPVersion: "v3.11.42"},
			expectedErr:  nil,
		},
	}

	for _, c := range cases {
		info, err := pv.GetPlatformInfo(c.discoverer, c.config)
		assert.Equal(t, c.expectedInfo, info, "mismatch in returned PlatformInfo")
		assert.Equal(t, c.expectedErr, err, "mismatch in returned error")
	}
}
