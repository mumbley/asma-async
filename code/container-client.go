package main

import (
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// createContainerClient creates a container client.
func (b *BlobArchiver) createContainerClient(connectionString, containerName string) (*container.Client, error) {
	client, err := azblob.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return nil, err
	}
	return client.ServiceClient().NewContainerClient(containerName), nil
}
