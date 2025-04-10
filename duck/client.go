/*
Copyright 2023 the original author or authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package duck

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsDuck returns true for types are not registered in the scheme and have a GVK set on the object
func IsDuck(obj runtime.Object, scheme *runtime.Scheme) bool {
	if _, _, err := scheme.ObjectKinds(obj); runtime.IsNotRegisteredError(err) {
		return obj.GetObjectKind() != nil && !obj.GetObjectKind().GroupVersionKind().Empty()
	}

	return false
}

// Convert from one object type to another. This operation is lossy depending on the specific types
// being converted.
func Convert(from runtime.Object, to runtime.Object) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(from)
	if err != nil {
		return fmt.Errorf("unable to convert from object: %w", err)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u, to); err != nil {
		return fmt.Errorf("unable to convert to object: %w", err)
	}
	return nil
}

type SchemeAccessor interface {
	Scheme() *runtime.Scheme
}

func NewDuckAwareAPIReaderWrapper(reader client.Reader, scheme SchemeAccessor) client.Reader {
	return &duckAwareAPIReaderWrapper{
		reader: reader,
		scheme: scheme,
	}
}

type duckAwareAPIReaderWrapper struct {
	reader client.Reader
	scheme SchemeAccessor
}

func (c *duckAwareAPIReaderWrapper) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if !IsDuck(obj, c.scheme.Scheme()) {
		return c.reader.Get(ctx, key, obj, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := c.reader.Get(ctx, key, u, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}

func (c *duckAwareAPIReaderWrapper) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if !IsDuck(list, c.scheme.Scheme()) {
		return c.reader.List(ctx, list, opts...)
	}

	u := &unstructured.UnstructuredList{}
	if err := Convert(list, u); err != nil {
		return err
	}
	if err := c.reader.List(ctx, u, opts...); err != nil {
		return err
	}
	return Convert(u, list)
}

func NewDuckAwareClientWrapper(client client.Client) client.Client {
	return &duckAwareClientWrapper{
		Reader: NewDuckAwareAPIReaderWrapper(client, client),
		client: client,
	}
}

type duckAwareClientWrapper struct {
	client.Reader
	client client.Client
}

func (c *duckAwareClientWrapper) Watch(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	ww, ok := c.client.(client.WithWatch)
	if !ok {
		panic(fmt.Errorf("unable to call Watch with wrapped client that does not implement client.WithWatch"))
	}

	if !IsDuck(list, c.Scheme()) {
		return ww.Watch(ctx, list, opts...)
	}

	u := &unstructured.UnstructuredList{}
	if err := Convert(list, u); err != nil {
		return nil, err
	}
	w, err := ww.Watch(ctx, u, opts...)
	if err != nil {
		return nil, err
	}
	if err := Convert(u, list); err != nil {
		return nil, err
	}
	return w, nil
}

func (c *duckAwareClientWrapper) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.Create(ctx, obj, opts...)
	}

	return fmt.Errorf("Create is not supported for the duck typed objects")
}

func (c *duckAwareClientWrapper) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.Update(ctx, obj, opts...)
	}

	return fmt.Errorf("Update is not supported for the duck typed objects, use Patch instead")
}

func (c *duckAwareClientWrapper) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.Patch(ctx, obj, patch, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := c.client.Patch(ctx, u, patch, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}

func (c *duckAwareClientWrapper) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.Delete(ctx, obj, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := c.client.Delete(ctx, u, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}

func (c *duckAwareClientWrapper) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.DeleteAllOf(ctx, obj, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := c.client.DeleteAllOf(ctx, u, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}

func (c *duckAwareClientWrapper) Status() client.SubResourceWriter {
	return &duckAwareSubResourceWriterWrapper{
		subResourceWriter: c.client.Status(),
		scheme:            c.client,
	}
}

func (c *duckAwareClientWrapper) SubResource(subResource string) client.SubResourceClient {
	return &duckAwareSubResourceClientWrapper{
		subResourceClient: c.client.SubResource(subResource),
		scheme:            c.client,
	}
}

func (c *duckAwareClientWrapper) Scheme() *runtime.Scheme {
	return c.client.Scheme()
}

func (c *duckAwareClientWrapper) RESTMapper() meta.RESTMapper {
	return c.client.RESTMapper()
}

func (c *duckAwareClientWrapper) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.GroupVersionKindFor(obj)
	}

	// TODO call upstream directly once kubernetes-sigs/controller-runtime#2434 lands
	objKind := obj.GetObjectKind()
	if objKind == nil || objKind.GroupVersionKind().Empty() {
		return schema.GroupVersionKind{}, fmt.Errorf("object must directly define APIVersion and Kind")
	}
	return objKind.GroupVersionKind(), nil
}

func (c *duckAwareClientWrapper) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	if !IsDuck(obj, c.Scheme()) {
		return c.client.IsObjectNamespaced(obj)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return false, err
	}
	return c.client.IsObjectNamespaced(u)
}

type duckAwareSubResourceWriterWrapper struct {
	subResourceWriter client.SubResourceWriter
	scheme            SchemeAccessor
}

func (w *duckAwareSubResourceWriterWrapper) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if !IsDuck(obj, w.scheme.Scheme()) {
		return w.subResourceWriter.Create(ctx, obj, subResource, opts...)
	}

	return fmt.Errorf("Create is not supported for the duck typed objects")
}

func (w *duckAwareSubResourceWriterWrapper) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if !IsDuck(obj, w.scheme.Scheme()) {
		return w.subResourceWriter.Update(ctx, obj, opts...)
	}

	return fmt.Errorf("Update is not supported for the duck typed objects, use Patch instead")
}

func (w *duckAwareSubResourceWriterWrapper) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if !IsDuck(obj, w.scheme.Scheme()) {
		return w.subResourceWriter.Patch(ctx, obj, patch, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := w.subResourceWriter.Patch(ctx, u, patch, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}

type duckAwareSubResourceClientWrapper struct {
	subResourceClient client.SubResourceClient
	scheme            SchemeAccessor
}

func (c *duckAwareSubResourceClientWrapper) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	if !IsDuck(obj, c.scheme.Scheme()) {
		return c.subResourceClient.Get(ctx, obj, subResource, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := c.subResourceClient.Get(ctx, u, subResource, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}

func (c *duckAwareSubResourceClientWrapper) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if !IsDuck(obj, c.scheme.Scheme()) {
		return c.subResourceClient.Create(ctx, obj, subResource, opts...)
	}

	return fmt.Errorf("Create is not supported for the duck typed objects")
}

func (c *duckAwareSubResourceClientWrapper) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if !IsDuck(obj, c.scheme.Scheme()) {
		return c.subResourceClient.Update(ctx, obj, opts...)
	}

	return fmt.Errorf("Update is not supported for the duck typed objects, use Patch instead")
}

func (c *duckAwareSubResourceClientWrapper) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if !IsDuck(obj, c.scheme.Scheme()) {
		return c.subResourceClient.Patch(ctx, obj, patch, opts...)
	}

	u := &unstructured.Unstructured{}
	if err := Convert(obj, u); err != nil {
		return err
	}
	if err := c.subResourceClient.Patch(ctx, u, patch, opts...); err != nil {
		return err
	}
	return Convert(u, obj)
}
