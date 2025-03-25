package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.uber.org/automaxprocs/maxprocs"
)

// Allowed operations
var (
	configFile        string = os.Getenv("BACKUP_CONFIG_FILE")
	allowedOperations        = []string{"backup", "restore", "backup-to-container", "download-tarfile", "upload-tarfile", "delete-all-blobs", "delete-tarfile", "count"}
)

// Check if the given operation is valid
func isValidOperation(op string) bool {
	for _, allowedOp := range allowedOperations {
		if op == allowedOp {
			return true
		}
	}
	return false
}

// Extracts the first argument that is a valid operation and returns the remaining arguments
func extractOperation(args []string) (string, []string) {
	for i, arg := range args {
		if isValidOperation(arg) {
			// Remove operation from args list
			return arg, append(args[:i], args[i+1:]...)
		}
	}
	return "", args
}

// Struct to store flag metadata
type FlagInfo struct {
	ShortName string
	LongName  string
	Desc      string
}

// List of flags with metadata
var flagsInfo = []FlagInfo{
	{"-c", "--connection-string", "Connection string"},
	{"-n", "--container-name", "Container name"},
	{"-p", "--prefix", "Prefix"},
	{"-z", "--compression", "Enable compression"},
	{"-P", "--path", "Path"},
	{"-t", "--tar-file-name", "Tar file name"},
	{"-tags", "--tar-file-tags", "Tags to identify and filter tar file: defaults to \"Name\"=\"BlobArchive\""},
	{"-o", "--overwrite", "Overwrite existing files"},
	{"-dc", "--destination-connection-string", "Destination connection string"},
	{"-dn", "--destination-container-name", "Destination container name"},
	{"-dp", "--destination-path", "Destination path"},
	{"-T", "--time", "Timestamp string - defaults to time.Now().Format(\"2006-01-02\")"},
	{"-b", "--batch-size", "Batch size of file each worker is allocated on backup - defaults to 25"},
	{"-w", "--workers", "Number of concurrent processes"},
}

// Prints enhanced help message
func printHelp() {
	fmt.Println("Usage: main.go <operation> [options]")
	fmt.Println("\nOperations:")
	for _, op := range allowedOperations {
		fmt.Printf("  %s\n", op)
	}
	fmt.Println("\nOptions:")
	for _, flag := range flagsInfo {
		fmt.Printf("  %-5s %-32s %s\n", flag.ShortName, flag.LongName, flag.Desc)
	}
	os.Exit(0)
}

// Implement the flag.Value interface for stringMapFlag
func (sm *StringMapFlag) String() string {
	parts := []string{}
	for key, value := range *sm {
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(parts, ",")
}

func (sm *StringMapFlag) Set(value string) error {
	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid format: %s", pair)
		}
		(*sm)[kv[0]] = kv[1]
	}
	return nil
}

func main() {

	fileConfig, err := NewConfigFromConfigFile(configFile)
	if err != nil {
		log.Printf("error parsing configuration file - falling back on parameters : %v", err)
	}

	// set the maximum parallel go routines to the number of available cores to the container
	unsetProc, err := maxprocs.Set()
	defer unsetProc()
	if err != nil {
		log.Fatalf("failed to set GOMAXPROCS: %v", err)
	}

	tarFileTags := StringMapFlag{}

	// Check if -h or --help is in args before parsing flags
	for _, arg := range os.Args {
		if arg == "-h" || arg == "--help" {
			printHelp()
		}
	}

	// Extract the operation
	operation, remainingArgs := extractOperation(os.Args[1:])
	if operation == "" {
		fmt.Println("Error: You must specify an operation: backup, restore, delete-all-blobs, or delete-tarfile")
		printHelp()
	}

	// Define flags
	connStr := flag.String("c", "", "Connection string (short: -c)")
	flag.StringVar(connStr, "connection-string", fileConfig.GetSourceAccountConnectString(), "Connection string")

	containerName := flag.String("n", "", "Container name (short: -n)")
	flag.StringVar(containerName, "container-name", fileConfig.GetSourceContainerName(), "Container name")

	prefix := flag.String("p", "", "Prefix (short: -p)")
	flag.StringVar(prefix, "prefix", "", "Prefix")

	compression := flag.Bool("z", false, "Enable compression (short: -z)")
	flag.BoolVar(compression, "compression", false, "Enable compression")

	path := flag.String("P", "", "Path (short: -P)")
	flag.StringVar(path, "path", "", "Path")

	tarFileName := flag.String("t", "", "Tar file name (short: -t)")
	flag.StringVar(tarFileName, "tar-file-name", "", "Tar file name")

	flag.Var(&tarFileTags, "tags", "Comma-separated key=value pairs (e.g., key1=value1,key2=value2)")

	overwrite := flag.Bool("o", false, "Overwrite existing files (short: -o)")
	flag.BoolVar(overwrite, "overwrite", false, "Overwrite existing files")

	destConnStr := flag.String("dc", "", "Destination connection string (short: -dc)")
	flag.StringVar(destConnStr, "destination-connection-string", fileConfig.GetDestAccountConnectString(), "Destination connection string")

	destContainerName := flag.String("dn", "", "Destination container name (short: -dn)")
	flag.StringVar(destContainerName, "destination-container-name", fileConfig.GetDestinationContainerName(), "Destination container name")

	destPath := flag.String("dp", "", "Destination path (short: -dp)")
	flag.StringVar(destPath, "destination-path", "", "Destination path")

	timeStr := flag.String("T", time.Now().Format("2006-01-02"), "Timestamp string (short: -T)")
	flag.StringVar(timeStr, "time", time.Now().Format("2006-01-02"), "Timestamp string")

	batchSize := flag.Int("b", 25, "Batch size (short: -b)")
	flag.IntVar(batchSize, "batch-size", 25, "Batch size - defaults to 25")

	workers := flag.Int("w", 8, "Workers (short: -w)")
	flag.IntVar(workers, "workers", 25, "Number of concurrent processes - defaults to 8")

	// flag.CommandLine.Parse(remainingArgs)

	// Override flag.CommandLine so we parse only remainingArgs
	// flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flag.CommandLine.Parse(remainingArgs)

	// Add a default flag if there are none set
	if len(tarFileTags) == 0 {
		tarFileTags["Name"] = "BlobArchive"
	}
	// Populate struct
	archiver := NewBlobArchiver(
		*connStr,
		*containerName,
		*prefix,
		*compression,
		*path,
		*tarFileName,
		tarFileTags,
		*overwrite,
		*destConnStr,
		*destContainerName,
		*destPath,
		*timeStr,
		*batchSize,
		*workers,
	)

	// Validate required flags
	if archiver.ConnectionString == "" {
		fmt.Println("Error: Connection string (-c or --connection-string) is required")
		flag.Usage() // Show help message
		os.Exit(1)   // Exit with an error code
	}

	if archiver.ContainerName == "" {
		fmt.Println("Error: Container Namme (-n or --container-name) is required")
		flag.Usage() // Show help message
		os.Exit(1)   // Exit with an error code
	}

	switch operation {

	case "backup":
		if err := archiver.StreamBlobsToTar(); err != nil {
			log.Fatal("error backing up storage container to tar file:", err)
		}
	case "backup-to-container":
		log.Print("checking for old tar backups")
		if err := deleteOldArchives(archiver.Path, []string{"tar", "tgz", "tar.tz"}); err != nil {
			log.Print("error trying to delete old tar files - continuing, but backup may fail due to lack of space - error :", err)
		}
		log.Print("beginning tar backup")
		if err := archiver.StreamBlobsToTar(); err != nil {
			log.Fatal("error backing up storage container to tar file:", err)
		}
		log.Print("backup complete")
		log.Print("archiving tar file to container")
		if err := archiver.CopyArchiveToStorageContainer(); err != nil {
			log.Fatal("error copying tarfile to storage container:", err)
		}
		log.Print("archive to container complete")
	case "upload-tarfile":
		if err := archiver.CopyArchiveToStorageContainer(); err != nil {
			log.Fatal("error copying tarfile to storage container:", err)
		}
	case "restore":
		if err := archiver.RestoreFromTarFile(); err != nil {
			log.Fatal(err)
		}
	case "delete-all-blobs":
		err := archiver.DeleteBatch()
		if err != nil {
			log.Fatalf("error deleting blobs from azure container %s - %v", archiver.ContainerName, err)
		}
	case "delete-tarfile":
		if err := archiver.DeleteTarFile(); err != nil {
			log.Fatalf("error deleting tar file %s - %v", archiver.TarFile(), err)
		}
	case "download-tarfile":
		log.Printf("downloading tarfile [%v] to [%v]", archiver.TarFileName, archiver.destinationPath)
		// need to check if the destination path is a file name or not.
		// this might end up as confusing
		// *** TODO *** decide on flags for restore
		if err := archiver.setDestinationTarFile(); err != nil {
			log.Fatal(err)
		}

		if err := archiver.DownloadBlobToBuffer(context.Background(),
			archiver.DestinationConnectionString,
			archiver.DestinationContainerName,
			archiver.TarFileName,
			archiver.destinationPath,
		); err != nil {
			log.Fatal(err)
		}
	case "count":
		log.Printf("counting blobs in container %v", archiver.ContainerName)
		n, err := archiver.CountBlobs()
		if err != nil {
			log.Fatal("unable to count blobs :", err)
		}
		log.Printf("number of blobs found : [%v]", n)
	default:
		log.Fatal("unkown error occured in initialising command args")

	}
}
