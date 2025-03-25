package main

import (
	"context"
	"fmt"
)

// CountBlobs counts the number of blobs in a container
func (b *BlobArchiver) CountBlobs() (int64, error) {
	ctx := context.Background()

	containerClient, err := b.createContainerClient(b.ConnectionString, b.ContainerName)
	if err != nil {
		return 0, fmt.Errorf("failed to create container client: %w", err)
	}

	var counter int64 = 0
	var pagecounter int64 = 0
	pager := containerClient.NewListBlobsFlatPager(nil)

	for pager.More() {

		pagecounter++
		page, err := pager.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to list blobs: %w", err)
		}
		for range page.Segment.BlobItems {
			counter++
		}
		fmt.Printf("[page : %d][count : %d]\n", pagecounter, counter)
	}
	return counter, nil
}
