package modal_test

import (
	"context"

	"github.com/perylemke/kaxa/internal/provider/modal"
)

// fakeClient is an in-memory modal SDK client.
type fakeClient struct {
	boxes    map[string]*modal.Record
	creates  int
	failWith error
}

func newFakeClient() *fakeClient {
	return &fakeClient{boxes: map[string]*modal.Record{}}
}

func (f *fakeClient) Create(_ context.Context, name string, labels []string) (modal.Record, error) {
	f.creates++
	if f.failWith != nil {
		return modal.Record{}, f.failWith
	}
	if _, ok := f.boxes[name]; ok {
		return modal.Record{}, modal.ErrAlreadyExists
	}
	rec := &modal.Record{ID: "sb-" + name, Name: name, Labels: labels}
	f.boxes[name] = rec
	return *rec, nil
}

func (f *fakeClient) List(context.Context) ([]modal.Record, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := make([]modal.Record, 0, len(f.boxes))
	for _, rec := range f.boxes {
		out = append(out, *rec)
	}
	return out, nil
}

func (f *fakeClient) Get(_ context.Context, name string) (modal.Record, error) {
	if f.failWith != nil {
		return modal.Record{}, f.failWith
	}
	rec, ok := f.boxes[name]
	if !ok {
		return modal.Record{}, modal.ErrNotFound
	}
	return *rec, nil
}

func (f *fakeClient) Terminate(_ context.Context, name string) error {
	if f.failWith != nil {
		return f.failWith
	}
	if _, ok := f.boxes[name]; !ok {
		return modal.ErrNotFound
	}
	delete(f.boxes, name)
	return nil
}

func (f *fakeClient) Close() {}
