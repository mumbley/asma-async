package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/schollz/progressbar/v3"
)

type batchProcessor struct {
	batch *container.BatchBuilder
	count int
}

func NewBatchProcessor(batch *container.BatchBuilder, batchLen int) *batchProcessor {
	return &batchProcessor{
		batch: batch,
		count: batchLen,
	}
}

// deleteAllBlobs deletes all blobs in the container.
func (b *BlobArchiver) DeleteAllBlobs() error {
	containerClient, err := b.createContainerClient(b.ConnectionString, b.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to create container client: %w", err)
	}

	pager := containerClient.NewListBlobsFlatPager(
		&container.ListBlobsFlatOptions{
			Prefix: &b.prefix,
		},
	)
	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return fmt.Errorf("failed to list blobs: %w", err)
		}

		for _, blobItem := range page.Segment.BlobItems {
			blobClient := containerClient.NewBlobClient(*blobItem.Name)
			if _, err := blobClient.Delete(context.Background(), nil); err != nil {
				return fmt.Errorf("failed to delete blob %s: %w", *blobItem.Name, err)
			}
		}
	}
	return nil
}

// deleteTarFile deletes the tar archive file.
func (b *BlobArchiver) DeleteTarFile() error {
	if err := os.Remove(b.TarFileName); err != nil {
		return fmt.Errorf("failed to delete tar file %s: %w", b.TarFileName, err)
	}
	fmt.Printf("Deleted tar file: %s\n", b.TarFileName)
	return nil
}

// DeleteBatch() **TODO** batch up delete requests and submit in batches
func (b *BlobArchiver) DeleteBatch() error {

	// pager has a limit of 5000 but the batcher limit is 256
	bar := progressbar.Default(-1, "blobs deleted")

	maxResults := 256
	maxPagerResults := int32(5000)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	childCtx, childCancel := context.WithCancel(ctx)
	defer childCancel()

	batchChan := make(chan *batchProcessor, 25)

	var wg sync.WaitGroup
	// create a container client
	containerClient, err := b.createContainerClient(b.ConnectionString, b.ContainerName)
	if err != nil {
		log.Fatal(err)
	}

	// creates a set of pages (up to 5000 blobs per page) to iterate through. The prefix is the top level pathname
	// although it is not really a path in the filesystem sense. It's more like a tag
	pager := containerClient.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix:     &b.prefix,
		MaxResults: &maxPagerResults,
	})

	batch, err := containerClient.NewBatchBuilder()
	if err != nil {
		return fmt.Errorf("failed to build batch: %w", err)
	}

	// run through each of the pages
	// batchChan := make(chan *container.BatchBuilder, 25)
	for i := 0; i < b.Workers; i++ {
		wg.Add(1)
		go func(ctx context.Context, workerNumber int) {
			defer wg.Done()
			// submit the batch
			for {
				select {
				case batcher, ok := <-batchChan:
					if !ok {
						// log.Printf("[%d] batch channel closed", workerNumber)
						return
					}
					if batcher == nil {
						// log.Printf("[%d] received nil batch", workerNumber)
						continue
					}
					// log.Printf("[%d] batch received", workerNumber)
					r, err := containerClient.SubmitBatch(ctx, batcher.batch, nil)
					if err != nil {
						log.Printf("failed to submit batch: %v", err)
					}
					for n, resp := range r.Responses {
						if err != nil {
							log.Printf("[%d][%v] - %v", n, *resp.BlobName, resp.Error)
						}
					}

					//log.Printf("[%d] processed batch", workerNumber)

				case <-ctx.Done():
					//log.Printf("[%d] worker cancelled", workerNumber)
					return
				}
			}
		}(childCtx, i)
	}
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list blobs: %w", err)
		}

		// Send blob names to the batcher as a delete request
		for k, blobItem := range page.Segment.BlobItems {
			bar.Add(1)
			err = batch.Delete(*blobItem.Name, nil)
			if err != nil {
				return fmt.Errorf("add delete request to batch: %w", err)
			}
			// log.Printf("processing batch item %d", k)
			// Check if the batch is full.
			if (k+1)%maxResults == 0 || (k+1) == len(page.Segment.BlobItems) {
				batchChan <- NewBatchProcessor(batch, k+1)
				// Reset the batch for the next set of blobs.
				batch, err = containerClient.NewBatchBuilder()
				if err != nil {
					return fmt.Errorf("failed to build batch: %w", err)
				}
			}
		}
	}

	log.Println("closing batch channel")
	close(batchChan)
	wg.Wait()
	return nil
}
