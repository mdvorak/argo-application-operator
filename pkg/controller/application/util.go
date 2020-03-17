package application

import (
	"context"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func getNoCache(c client.Client, ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	// This a HACK, to bypass caching, which is broken when accessing object outside namespace
	clientReader := c.(*client.DelegatingClient).Reader.(*client.DelegatingReader).ClientReader
	return clientReader.Get(ctx, key, obj)
}
