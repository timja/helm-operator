module github.com/fluxcd/helm-operator

go 1.13

require (
	github.com/bugsnag/bugsnag-go v1.5.3 // indirect
	github.com/deislabs/oras v0.7.0 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-kit/kit v0.9.0
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.3.0
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/gorilla/handlers v1.4.2 // indirect
	github.com/gorilla/mux v1.7.1
	github.com/gosuri/uitable v0.0.3 // indirect
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/instrumenta/kubeval v0.0.0-20190804145309-805845b47dfc
	github.com/json-iterator/go v1.1.7 // indirect
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/spf13/pflag v1.0.3
	github.com/stretchr/testify v1.3.0
	github.com/weaveworks/flux v0.0.0-20190729133003-c78ccd3706b5
	github.com/yvasiyarov/gorelic v0.0.7 // indirect
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45 // indirect
	google.golang.org/grpc v1.20.1
	gopkg.in/yaml.v2 v2.2.2
	helm.sh/helm v3.0.0-beta.3+incompatible
	k8s.io/api v0.0.0
	k8s.io/apiextensions-apiserver v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/helm v2.14.3+incompatible
	k8s.io/klog v0.3.3
	k8s.io/kubernetes v1.15.4
	k8s.io/utils v0.0.0-20190712204705-3dccf664f023 // indirect
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/yaml v1.1.0
)

// this is required to avoid
//    github.com/docker/distribution@v0.0.0-00010101000000-000000000000: unknown revision 000000000000
// because flux also replaces it, and we depend on flux
replace (
	github.com/docker/distribution => github.com/2opremio/distribution v0.0.0-20190419185413-6c9727e5e5de
	github.com/docker/docker => github.com/docker/docker v0.7.3-0.20190327010347-be7ac8be2ae0
)

// The following pin these libs to `kubernetes-1.15.x` (by initially
// giving the version as that tag, and letting go mod fill in its idea of
// the version).
// The libs are thereby kept compatible with client-go v12, which is
// itself compatible with Kubernetes 1.15.
replace (
	k8s.io/api => k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190918201827-3de75813f604
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190918200908-1e17798da8c1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190918202139-0b14c719ca62
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190918203125-ae665f80358a
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190918202959-c340507a5d48
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190918200425-ed2f0867c778
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190817025403-3ae76f584e79
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20190918203248-97c07dcbb623
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190918201136-c3a845f1fbb2
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20190918202837-c54ce30c680e
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20190918202429-08c8357f8e2d
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20190918202713-c34a54b3ec8e
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20190602132728-7075c07e78bf
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20190918202550-958285cf3eef
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190918203421-225f0541b3ea
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190918202012-3c1ca76f5bda
	k8s.io/node-api => k8s.io/node-api v0.0.0-20190918203548-2c4c2679bece
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190918201353-5cc279503896
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.0.0-20190918202305-ed68a9f09ae1
	k8s.io/sample-controller => k8s.io/sample-controller v0.0.0-20190918201537-fabef0de90df
)
