package storage

import (
	"bytes"
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"io/ioutil"
)

type MinioStorage struct {
	bucket  string
	client  *minio.Client
	context context.Context
}

func New(endpoint, bucket, accessKey, secretKey string) (*MinioStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: true,
	})
	if err != nil {
		return nil, err
	}
	return &MinioStorage{bucket: bucket, client: client, context: context.Background()}, nil
}

func (ms *MinioStorage) Exists(path string) bool {
	if _, err := ms.client.StatObject(ms.context, ms.bucket, path, minio.StatObjectOptions{}); err != nil {
		return false
	}
	return true
}

func (ms *MinioStorage) ReadFile(path string) ([]byte, error) {
	object, err := ms.client.GetObject(ms.context, ms.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(object)
}

func (ms *MinioStorage) WriteFile(path string, data []byte) error {
	reader := bytes.NewReader(data)
	if _, err := ms.client.PutObject(ms.context, ms.bucket, path, reader, reader.Size(), minio.PutObjectOptions{}); err != nil {
		return err
	}
	return nil
}

func (ms *MinioStorage) WriteFileWithReader(path string, data []byte, progress io.Reader) error {
	reader := bytes.NewReader(data)
	if _, err := ms.client.PutObject(ms.context, ms.bucket, path, reader, reader.Size(), minio.PutObjectOptions{Progress: progress}); err != nil {
		return err
	}
	return nil
}

func (ms *MinioStorage) Remove(name string) error {
	return ms.client.RemoveObject(ms.context, ms.bucket, name, minio.RemoveObjectOptions{ForceDelete: true})
}

func (ms *MinioStorage) RemoveAll(path string) error {
	for object := range ms.client.ListObjects(ms.context, ms.bucket, minio.ListObjectsOptions{
		Prefix:    path,
		Recursive: true,
	}) {
		if object.Err != nil {
			return object.Err
		}
		if err := ms.client.RemoveObject(ms.context, ms.bucket, object.Key, minio.RemoveObjectOptions{ForceDelete: true}); err != nil {
			return nil
		}
	}
	return nil
}

func (ms *MinioStorage) Walk(root string, fn func(path string, err error) error) error {
	for objects := range ms.client.ListObjects(ms.context, ms.bucket, minio.ListObjectsOptions{
		Prefix:    root,
		Recursive: true,
	}) {
		if err := fn(objects.Key, objects.Err); err != nil {
			return err
		}
	}
	return nil
}
