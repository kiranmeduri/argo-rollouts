package istio

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func DoesAppMeshExist(dynamicClient dynamic.Interface, namespace string) bool {
	_, err := dynamicClient.Resource(GetAppMeshVirtualServiceGVR()).Namespace(namespace).List(context.TODO(), metav1.ListOptions{Limit: 1})
	if err != nil {
		return false
	}
	return true
}

func GetAppMeshVirtualServiceGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "appmesh.k8s.aws",
		Version:  "v1beta2",
		Resource: "virtualservices",
	}
}

func GetAppMeshVirtualRouterGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "appmesh.k8s.aws",
		Version:  "v1beta2",
		Resource: "virtualrouters",
	}
}

func GetAppMeshVirtualNodeGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "appmesh.k8s.aws",
		Version:  "v1beta2",
		Resource: "virtualnodes",
	}
}
