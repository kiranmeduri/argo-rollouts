package appmesh

import (
	"strings"
	"testing"

	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	testutil "github.com/argoproj/argo-rollouts/test/util"
	"github.com/argoproj/argo-rollouts/utils/record"
	unstructuredutil "github.com/argoproj/argo-rollouts/utils/unstructured"
	"github.com/tj/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func fakeRollout() *v1alpha1.Rollout {
	return &v1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rollout",
			Namespace: "myns",
		},
		Spec: v1alpha1.RolloutSpec{
			Strategy: v1alpha1.RolloutStrategy{
				Canary: &v1alpha1.CanaryStrategy{
					TrafficRouting: &v1alpha1.RolloutTrafficRouting{
						AppMesh: &v1alpha1.AppMeshTrafficRouting{
							VirtualService: &v1alpha1.AppMeshVirtualService{
								Name:   "mysvc",
								Routes: []string{"primary"},
							},
							VirtualNodeGroup: &v1alpha1.AppMeshVirtualNodeGroup{
								CanaryVirtualNodeRef: &v1alpha1.AppMeshVirtualNodeReference{
									Name: "mysvc-canary-vn",
								},
								StableVirtualNodeRef: &v1alpha1.AppMeshVirtualNodeReference{
									Name: "mysvc-stable-vn",
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestSetWeightWithMissingVsvc(t *testing.T) {
	client := testutil.NewFakeDynamicClient()
	ro := fakeRollout()
	cfg := ReconcilerConfig{
		Rollout:  ro,
		Client:   client,
		Recorder: record.NewFakeEventRecorder(),
	}
	r := NewReconciler(cfg)
	err := r.SetWeight(0)
	assert.EqualError(t, err, ErrVirtualServiceMissing)
	actions := client.Actions()
	assert.Len(t, actions, 1)
	assert.True(t, actions[0].Matches("get", "virtualservices"))
}

func TestSetWeightVsvcWithVnodeProvider(t *testing.T) {
	vsvc := unstructuredutil.StrToUnstructuredUnsafe(vsvcWithVnode)
	client := testutil.NewFakeDynamicClient(vsvc)
	ro := fakeRollout()
	cfg := ReconcilerConfig{
		Rollout:  ro,
		Client:   client,
		Recorder: record.NewFakeEventRecorder(),
	}
	r := NewReconciler(cfg)
	err := r.SetWeight(0)
	assert.EqualError(t, err, ErrVirtualServiceNotUsingVirtualRouter)
	actions := client.Actions()
	assert.Len(t, actions, 1)
	assert.True(t, actions[0].Matches("get", "virtualservices"))
}

func TestSetWeightForVsvcWithMissingVrouter(t *testing.T) {
	vsvc := unstructuredutil.StrToUnstructuredUnsafe(vsvcWithVrouter)
	client := testutil.NewFakeDynamicClient(vsvc)
	cfg := ReconcilerConfig{
		Rollout:  fakeRollout(),
		Client:   client,
		Recorder: record.NewFakeEventRecorder(),
	}
	r := NewReconciler(cfg)
	err := r.SetWeight(50)
	assert.EqualError(t, err, ErrVirtualRouterMissing)
	actions := client.Actions()
	assert.Len(t, actions, 2)
	assert.True(t, actions[0].Matches("get", "virtualservices"))
	assert.True(t, actions[1].Matches("get", "virtualrouters"))
}

func TestSetWeightForVsvcWithVrouter(t *testing.T) {
	type args struct {
		vsvc      *unstructured.Unstructured
		vrouter   *unstructured.Unstructured
		routeType string
		rollout   *v1alpha1.Rollout
	}

	fixtures := []struct {
		name string
		args args
	}{
		{
			name: "http",
			args: args{
				vsvc:      unstructuredutil.StrToUnstructuredUnsafe(vsvcWithVrouter),
				vrouter:   unstructuredutil.StrToUnstructuredUnsafe(baselineVrouterWithHTTPRoutes),
				routeType: "httpRoute",
				rollout:   fakeRollout(),
			},
		},
		{
			name: "tcp",
			args: args{
				vsvc:      unstructuredutil.StrToUnstructuredUnsafe(vsvcWithVrouter),
				vrouter:   unstructuredutil.StrToUnstructuredUnsafe(baselineVrouterWithTCPRoutes),
				routeType: "tcpRoute",
				rollout:   fakeRollout(),
			},
		},
		{
			name: "http2",
			args: args{
				vsvc:      unstructuredutil.StrToUnstructuredUnsafe(vsvcWithVrouter),
				vrouter:   unstructuredutil.StrToUnstructuredUnsafe(baselineVrouterWithHTTP2Routes),
				routeType: "http2Route",
				rollout:   fakeRollout(),
			},
		},
		{
			name: "grpc",
			args: args{
				vsvc:      unstructuredutil.StrToUnstructuredUnsafe(vsvcWithVrouter),
				vrouter:   unstructuredutil.StrToUnstructuredUnsafe(baselineVrouterWithGRPCRoutes),
				routeType: "grpcRoute",
				rollout:   fakeRollout(),
			},
		},
	}

	for _, wantUpdate := range []bool{true, false} {
		for _, fixture := range fixtures {
			client := testutil.NewFakeDynamicClient(fixture.args.vsvc, fixture.args.vrouter)
			cfg := ReconcilerConfig{
				Rollout:  fixture.args.rollout,
				Client:   client,
				Recorder: record.NewFakeEventRecorder(),
			}
			r := NewReconciler(cfg)
			desiredWeight := 0
			if wantUpdate {
				desiredWeight = 55
			}
			err := r.SetWeight(int32(desiredWeight))
			assert.Nil(t, err)
			actions := client.Actions()
			if wantUpdate {
				assert.Len(t, actions, 3)
				assert.True(t, actions[0].Matches("get", "virtualservices"))
				assert.True(t, actions[1].Matches("get", "virtualrouters"))
				assert.True(t, actions[2].Matches("update", "virtualrouters"))
				assertSetWeightAction(t, actions[2], int64(desiredWeight), fixture.args.routeType)
			} else {
				assert.Len(t, actions, 2)
				assert.True(t, actions[0].Matches("get", "virtualservices"))
				assert.True(t, actions[1].Matches("get", "virtualrouters"))
			}
		}
	}
}

func TestUpdateHash(t *testing.T) {
	type args struct {
		inputCanaryHash    string
		inputStableHash    string
		existingCanaryHash string
		existingStableHash string
		expectedCanaryHash string
		expectedStableHash string
		rollout            *v1alpha1.Rollout
	}

	fixtures := []struct {
		name string
		args args
	}{
		{
			name: "with no existing hashes",
			args: args{
				inputCanaryHash:    "canary-new",
				expectedCanaryHash: "canary-new",
				inputStableHash:    "stable-new",
				expectedStableHash: "stable-new",
				rollout:            fakeRollout(),
			},
		},
		{
			name: "with different existing hashes",
			args: args{
				inputCanaryHash:    "canary-new",
				existingCanaryHash: "canary-old",
				expectedCanaryHash: "canary-new",
				inputStableHash:    "stable-new",
				existingStableHash: "stable-old",
				expectedStableHash: "stable-new",
				rollout:            fakeRollout(),
			},
		},
		{
			name: "with existing hashes cleared",
			args: args{
				inputCanaryHash:    "",
				existingCanaryHash: "canary-old",
				expectedCanaryHash: defaultCanaryHash,
				inputStableHash:    "",
				existingStableHash: "stable-old",
				expectedStableHash: defaultStableHash,
				rollout:            fakeRollout(),
			},
		},
		{
			name: "with canaryHash == stableHash",
			args: args{
				inputCanaryHash:    "12345",
				existingCanaryHash: "canary-old",
				expectedCanaryHash: defaultCanaryHash,
				existingStableHash: "stable-old",
				inputStableHash:    "12345",
				expectedStableHash: "12345",
				rollout:            fakeRollout(),
			},
		},
	}

	for _, fixture := range fixtures {
		canaryVnode := createVnodeWithHash(baselineCanaryVnode, fixture.args.existingCanaryHash)
		stableVnode := createVnodeWithHash(baselineStableVnode, fixture.args.existingStableHash)
		client := testutil.NewFakeDynamicClient(canaryVnode, stableVnode)
		cfg := ReconcilerConfig{
			Rollout:  fixture.args.rollout,
			Client:   client,
			Recorder: record.NewFakeEventRecorder(),
		}
		r := NewReconciler(cfg)

		err := r.UpdateHash(fixture.args.inputCanaryHash, fixture.args.inputStableHash)
		assert.Nil(t, err)
		actions := client.Actions()
		assert.Len(t, actions, 4)
		assert.True(t, actions[0].Matches("get", "virtualnodes"))
		assert.True(t, actions[1].Matches("update", "virtualnodes"))
		assertUpdateHashAction(t, actions[1], fixture.args.expectedStableHash)
		assert.True(t, actions[2].Matches("get", "virtualnodes"))
		assert.True(t, actions[3].Matches("update", "virtualnodes"))
		assertUpdateHashAction(t, actions[3], fixture.args.expectedCanaryHash)
	}
}

func TestUpdateHashWithVirtualNodeMissingMatchLabels(t *testing.T) {
	canaryVnode := unstructuredutil.StrToUnstructuredUnsafe(baselineCanaryVnode)
	unstructured.SetNestedMap(canaryVnode.Object, make(map[string]interface{}), "spec", "podSelector")
	stableVnode := unstructuredutil.StrToUnstructuredUnsafe(baselineStableVnode)
	unstructured.SetNestedMap(stableVnode.Object, make(map[string]interface{}), "spec", "podSelector")
	client := testutil.NewFakeDynamicClient(canaryVnode, stableVnode)
	cfg := ReconcilerConfig{
		Rollout:  fakeRollout(),
		Client:   client,
		Recorder: record.NewFakeEventRecorder(),
	}
	r := NewReconciler(cfg)

	canaryHash := "canary-new"
	stableHash := "stable-new"
	err := r.UpdateHash(canaryHash, stableHash)
	assert.Nil(t, err)
	actions := client.Actions()
	assert.Len(t, actions, 4)
	assert.True(t, actions[0].Matches("get", "virtualnodes"))
	assert.True(t, actions[1].Matches("update", "virtualnodes"))
	assertUpdateHashAction(t, actions[1], stableHash)
	assert.True(t, actions[2].Matches("get", "virtualnodes"))
	assert.True(t, actions[3].Matches("update", "virtualnodes"))
	assertUpdateHashAction(t, actions[3], canaryHash)
}

func createVnodeWithHash(baselineVnode string, hash string) *unstructured.Unstructured {
	vnode := unstructuredutil.StrToUnstructuredUnsafe(baselineVnode)
	ml, _ := getPodSelectorMatchLabels(vnode)
	ml[v1alpha1.DefaultRolloutUniqueLabelKey] = hash
	setPodSelectorMatchLabels(vnode, ml)
	return vnode
}

func assertUpdateHashAction(t *testing.T, action k8stesting.Action, hash string) {
	updateAction := action.(k8stesting.UpdateAction)
	uVnode, err := runtime.DefaultUnstructuredConverter.ToUnstructured(updateAction.GetObject())
	assert.Nil(t, err)
	matchLabels, found, err := unstructured.NestedMap(uVnode, "spec", "podSelector", "matchLabels")
	assert.True(t, found, "Virtual-node's podSelector is missing matchLabels")
	assert.Nil(t, err)
	assert.Equal(t, matchLabels[v1alpha1.DefaultRolloutUniqueLabelKey].(string), hash)
}

func assertSetWeightAction(t *testing.T, action k8stesting.Action, desiredWeight int64, routeType string) {
	updateAction := action.(k8stesting.UpdateAction)
	uVr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(updateAction.GetObject())
	assert.Nil(t, err)
	routesI, _, err := unstructured.NestedMap(uVr, "spec", "routes")
	for _, routeI := range routesI {
		route, _ := routeI.(map[string]interface{})
		weightedTargetsI, found, err := unstructured.NestedSlice(route, routeType, "action", "weightedTargets")
		assert.Nil(t, err)
		assert.True(t, found, "Did not find weightedTargets in route")
		assert.Len(t, weightedTargetsI, 2)
		for _, wtI := range weightedTargetsI {
			wt, _ := wtI.(map[string]interface{})
			vnodeName, _, err := unstructured.NestedString(wt, "virtualNodeRef", "name")
			assert.Nil(t, err)
			weight, err := toInt64(wt["weight"])
			assert.Nil(t, err)
			if strings.Contains(vnodeName, "canary") {
				assert.Equal(t, weight, int64(desiredWeight))
			} else {
				assert.Equal(t, weight, int64(100-desiredWeight))
			}
		}
	}
}

const vsvcWithVnode = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualService
metadata:
  name: mysvc
  namespace: myns
spec:
  awsName: mysvc.myns.svc.cluster.local
  provider:
    virtualNode:
      virtualNodeRef:
        name: mysvc-vnode`

const vsvcWithVrouter = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualService
metadata:
  namespace: myns
  name: mysvc
spec:
  awsName: mysvc.myns.svc.cluster.local
  provider:
    virtualRouter:
      virtualRouterRef:
        name: mysvc-vrouter`

const baselineVrouterWithHTTPRoutes = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualRouter
metadata:
  namespace: myns
  name: mysvc-vrouter
spec:
  listeners:
    - portMapping:
        port: 8080
        protocol: http
  routes:
    - name: primary
      httpRoute:
        match:
          prefix: /
        action:
          weightedTargets:
            - virtualNodeRef:
                name: mysvc-canary-vn
              weight: 0
            - virtualNodeRef:
                name: mysvc-stable-vn
              weight: 100`

const baselineVrouterWithGRPCRoutes = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualRouter
metadata:
  namespace: myns
  name: mysvc-vrouter
spec:
  listeners:
    - portMapping:
        port: 8080
        protocol: http
  routes:
    - name: primary
      grpcRoute:
        match:
          methodName: GetItem
          serviceName: MySvc
        action:
          weightedTargets:
            - virtualNodeRef:
                name: mysvc-canary-vn
              weight: 0
            - virtualNodeRef:
                name: mysvc-stable-vn
              weight: 100`

const baselineVrouterWithHTTP2Routes = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualRouter
metadata:
  namespace: myns
  name: mysvc-vrouter
spec:
  listeners:
    - portMapping:
        port: 8080
        protocol: http
  routes:
    - name: primary
      http2Route:
        match:
          prefix: /
        action:
          weightedTargets:
            - virtualNodeRef:
                name: mysvc-canary-vn
              weight: 0
            - virtualNodeRef:
                name: mysvc-stable-vn
              weight: 100`

const baselineVrouterWithTCPRoutes = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualRouter
metadata:
  namespace: myns
  name: mysvc-vrouter
spec:
  listeners:
    - portMapping:
        port: 8080
        protocol: http
  routes:
    - name: primary
      tcpRoute:
        action:
          weightedTargets:
            - virtualNodeRef:
                name: mysvc-canary-vn
              weight: 0
            - virtualNodeRef:
                name: mysvc-stable-vn
              weight: 100`

const baselineCanaryVnode = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualNode
metadata:
  namespace: myns
  name: mysvc-canary-vn
spec:
  podSelector:
    matchLabels:
      app: mysvc-pod
  listeners:
    - portMapping:
        port: 8080
        protocol: http
  serviceDiscovery:
    dns:
      hostname: mysvc.myns.svc.cluster.local`

const baselineStableVnode = `
apiVersion: appmesh.k8s.aws/v1beta2
kind: VirtualNode
metadata:
  namespace: myns
  name: mysvc-stable-vn
spec:
  podSelector:
    matchLabels:
      app: mysvc-pod
  listeners:
    - portMapping:
        port: 8080
        protocol: http
  serviceDiscovery:
    dns:
      hostname: mysvc.myns.svc.cluster.local`
