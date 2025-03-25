package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/schollz/progressbar/v3"
	// "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/internal/exported"
)

// StreamBlobsToTar downloads blobs, archives them into a tar file, and uses goroutines for concurrency.
func (b *BlobArchiver) StreamBlobsToTar() error {
	var bar = progressbar.Default(-1, "downloading blobs")

	// Create a container client for source storage account
	containerClient, err := b.createContainerClient(b.ConnectionString, b.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to create container client: %w", err)
	}

	// Create directory if it doesn't exist
	if b.Path != "" {
		if err := os.MkdirAll(b.Path, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Open tar file for writing
	tarFile, err := os.Create(b.TarFile())
	if err != nil {
		return fmt.Errorf("failed to create tar file: %w", err)
	}
	defer tarFile.Close()

	var tarWriter *tar.Writer
	var gzipWriter *gzip.Writer

	if b.Compression {
		// maximise compression. I might make this switchable later on but, at the moment, that just feels like Yak shaving
		gzipWriter, err = gzip.NewWriterLevel(tarFile, 9)
		if err != nil {
			return fmt.Errorf("failed to create gzip writer : %v", err)
		}
		defer gzipWriter.Close()
		tarWriter = tar.NewWriter(gzipWriter)
	} else {
		tarWriter = tar.NewWriter(tarFile)
	}
	defer tarWriter.Close()

	// Mutex to protect tar file access
	var tarMutex sync.Mutex

	// Wait group to synchronize goroutines
	var wg sync.WaitGroup

	// Channel to send batches of blobs
	blobChan := make(chan []*container.BlobItem, b.BatchSize)

	// Start a worker pool to process blobs concurrently
	numWorkers := b.Workers // Number of goroutines for downloading blobs
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range blobChan {
				if err := b.processBlobBatch(batch, containerClient, tarWriter, &tarMutex); err != nil {
					fmt.Printf("Error processing blob batch: %v\n", err)
				}
			}
		}()
	}

	// Read blobs from Azure and send them in batches to the channel
	pager := containerClient.NewListBlobsFlatPager(nil)
	batch := make([]*container.BlobItem, 0, b.BatchSize)

	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return fmt.Errorf("failed to list blobs: %w", err)
		}

		for _, blobItem := range page.Segment.BlobItems {
			batch = append(batch, blobItem)
			batchLen := len(batch)
			if batchLen >= b.BatchSize {
				blobChan <- batch
				batch = make([]*container.BlobItem, 0, b.BatchSize) // Reset batch
				bar.Add(batchLen)
			}
		}
	}
	// Process remaining blobs in the last batch
	if len(batch) > 0 {
		// add the last count of batch to the progress bar
		bar.Add(len(batch))
		blobChan <- batch
	}
	close(blobChan) // Close the channel to signal workers to stop
	wg.Wait()       // Wait for all workers to finish

	fmt.Printf("Blobs archived to %s\n", b.TarFile())
	return nil
}

// processBlobBatch downloads blobs in a batch and adds them to the tar archive.
func (b *BlobArchiver) processBlobBatch(batch []*container.BlobItem, containerClient *container.Client, tarWriter *tar.Writer, tarMutex *sync.Mutex) error {
	for _, blobItem := range batch {
		// fmt.Printf("adding blob: %s\n", *blobItem.Name)

		// Create a blob client for the current blob
		blobClient := containerClient.NewBlobClient(*blobItem.Name)

		// Download the blob
		get, err := blobClient.DownloadStream(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("failed to download blob %s: %w", *blobItem.Name, err)
		}
		defer get.Body.Close()

		// Write blob to tar file
		header := &tar.Header{
			Name:    *blobItem.Name,
			Size:    *blobItem.Properties.ContentLength,
			ModTime: *blobItem.Properties.LastModified,
			Uid:     1000,
			Gid:     1000,
			Mode:    0600,
		}

		// Use mutex to protect the tarWriter
		tarMutex.Lock()
		if err := tarWriter.WriteHeader(header); err != nil {
			tarMutex.Unlock()
			return fmt.Errorf("failed to write tar header for %s: %w", *blobItem.Name, err)
		}
		if _, err := io.Copy(tarWriter, get.Body); err != nil {
			tarMutex.Unlock()
			return fmt.Errorf("failed to write blob %s to tar: %w", *blobItem.Name, err)
		}
		tarMutex.Unlock()
	}
	return nil
}

// CopyArchiveToStorageContainer uploads the tar file to the destination storage container.
func (b *BlobArchiver) CopyArchiveToStorageContainer() error {
	if b.DestinationConnectionString == "" || b.DestinationContainerName == "" {
		return fmt.Errorf("destination connection string or container name not provided")
	}
	tf := b.TarFile()

	ctx := context.Background()

	blockBlobClient, err := blockblob.NewClientFromConnectionString(
		b.DestinationConnectionString,
		b.DestinationContainerName,
		fmt.Sprintf("%s/%s", b.destinationPrefix(), b.TarFile()),
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Open tar file for reading
	tarFile, err := os.Open(tf)
	if err != nil {
		return fmt.Errorf("failed to open tar file: %w", err)
	}
	defer tarFile.Close()

	fileStat, err := tarFile.Stat()
	fileSize := fileStat.Size()
	if err != nil {
		return (err)
	}

	log.Print("streaming buffer to storage container")
	response, err := blockBlobClient.UploadFile(context.Background(), tarFile,
		&blockblob.UploadBufferOptions{
			BlockSize:   int64(Divisor(fileSize)),
			Concurrency: uint16(b.Workers),
		},
	)
	// response is required in the above upload, this prevents a unuesd error. This is a bit hackey but wen don't need anything
	// from the response but it cannot be underscored out in the request for some reason
	_ = response
	if err != nil {
		return (err)
	}
	log.Print("Azure upload complete")

	// Adding a tag to the blob. This will help if you need to set a lifecycle policy, based on tags. Azure does not have a wildcard
	// filter in lifecycle management so tags is the best option for filtering
	_, err = blockBlobClient.SetTags(ctx, b.TarFileTags, &blob.SetTagsOptions{})
	if err != nil {
		log.Printf("error: unable to add tags. If lifecycle management is enabled, this file may not be included : %v\n", err)
	}
	log.Print("tags generated")
	return nil

}

// DownloadBlob sequentially downloads a named blob to a destination path
func (b *BlobArchiver) DownloadBlob(
	ctx context.Context,
	connectionString string,
	containerName string,
	blobName string,
	destination string,
) error {

	containerClient, err := b.createContainerClient(connectionString, containerName)
	if err != nil {
		return fmt.Errorf("failed to create container client: %w", err)
	}
	blobClient := containerClient.NewBlobClient(blobName)

	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return fmt.Errorf("unable to create destination file [%s]: %v", destination, err)
	}
	defer f.Close()

	properties, err := blobClient.GetProperties(ctx, nil)
	blobSize := *properties.ContentLength
	if err != nil {
		return (err)
	}

	n, err := blobClient.DownloadFile(context.Background(), f, &blob.DownloadFileOptions{
		BlockSize:   Divisor(blobSize),
		Concurrency: uint16(b.Workers),
	})
	if err != nil {
		log.Fatal("error downloading blob :", err)
	}
	log.Printf("[%d] bytes downloaded\n", n)

	return nil
}

func (b *BlobArchiver) DownloadBlobToBuffer(
	ctx context.Context,
	connectionString string,
	containerName string,
	blobName string,
	destination string,
) error {
	containerClient, err := b.createContainerClient(connectionString, containerName)
	if err != nil {
		return fmt.Errorf("failed to create container client: %w", err)
	}
	blobClient := containerClient.NewBlobClient(blobName)

	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return fmt.Errorf("unable to create destination file [%s]: %v", destination, err)
	}
	defer f.Close()
	properties, err := blobClient.GetProperties(ctx, nil)
	blobSize := *properties.ContentLength
	if err != nil {
		return (err)
	}
	var bufferLen int64 = 104057600
	count := bufferLen
	fmt.Println("blobsize", blobSize)
	var offset int64 = 0
	bar := progressbar.DefaultBytes(
		blobSize,
		"downloading",
	)
	for {
		if offset+count > blobSize {
			count = 0
		}
		if offset >= blobSize {
			break
		}
		response, err := blobClient.DownloadStream(context.Background(), &blob.DownloadStreamOptions{
			Range: blob.HTTPRange{
				Offset: offset,
				Count:  count,
			},
		})
		if err != nil {
			return fmt.Errorf("error reading from container stream : %v", err)
		}
		_, err = io.Copy(io.MultiWriter(f, bar), response.Body)
		if err != nil {
			log.Fatal("error downloading blob :", err)
		}
		offset += bufferLen
	}

	return nil
}
