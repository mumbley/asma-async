# Azure Storagee Container Archive
## Backup and restore Azure storage containers, using go routines to help bypass Azure throttling

  ## Tl;Dr 
  If you are already using this tool, are in the middle of an incident and need to recover a storage archive follow these steps:
  1. Download the Helm chart
  2. `cd <chart path>`
  3. `helm upgrade --install azure-backup --namespace azure-backup --create-namespace --set global.task=pod .` This changes the deployment from a cronjob to a pod. After a short time, the pod will become available and you can attach.
  4. `kubectl exec -ti azure-archive -n azure-archive -- ash` at this point you should get a prompt
  5. `ls -l /mnt/backup` this should display the last backup taken of the datasource you want to restore
  6. `/mnt/app/./azarchive restore -t /mnt/backup/<tarfile>` this will start the restore process. You should see a progress bar displaying the progress of the restore process.
  
 restoring tarfile  97% |█████████████████████████████████████████████  | (495/507 MB, 52 MB/s) [7s:0s]
 2025/03/13 13:55:32 Tar file restored to the source container `<storage container>`

***WARNING Do not forget to change the deployment back to `crontab` from `pod`***

Failing to do so will prevent further automated backups
`
helm upgrade --install azure-backup --namespace azure-backup --create-namespace --set global.task=cron .`

## Features
The tool has the following capabilities:

Operations:
|function|description  |
|--|--|
|backup|backup a storage container to a destination storage container|
  |restore|restore data to a storage container from a backup|
  |backup-to-container|backup to a tar archive file and then copy that file to a storage container|
  |download-tarfile|download a tarfile from a storage container to a path|
  |upload-tarfile|upload a tarfile from a path to a storage container|
  |delete-all-blobs|delete all blobs in the source storage container|
  |delete-tarfile|delete a tarfile from a path|
  |count|count the number of files in a storage container|

Options:
|option|long option|description|
|--|--|--|
 | -c|    --connection-string|source storage account connection string
 | -n|    --container-name|source container name
 | -p|    --prefix|prefix : source data filter|
  |-z|    --compression|Enable compression dafault is `false`
  |-P|    --path|destination file path|
  |-t|    --tar-file-name|tar file name
  |-o|    --overwrite|overwrite existing files|
  |-dc|   --destination-connection-string|  Destination storage account connection string
  |-dn|   --destination-container-name |    destination container name|
  -dp|   --destination-path|               Destination path
  -T |   --time|                           timestamp string - defaults to time.Now().Format("2006-01-02")|
  |-b|    --batch-size|                     batch number of files each worker is allocated - defaults to 25|
  |-w|    --workers |                       number of concurrent processes - defaults to number of cores|

## Configuration file
There is a configuration file, generated from a kubernetes secret. The file can be found mounted at `/etc/azure-storage-manager/azure-storage-manager-keys`. the configuration file contains key-pairs as follows:
```
sourceContainerName: <sourceContainerName>
sourceAccountConnectString: <sourceConnectionString>
destinationContainerName: <destinationContainerName>
destAccountConnectString: <destinationConnectionString>
```
The container names and connection strings can be found in the Azure console in the following paths:

Container name: `home->storage accounts->storage account name->containers`

Connect string `home->storage accounts->storage account name->security+networking->access keys`

## Example commands
`/mnt/app/azarchive backup-to-container -dp testblobstore -P /mnt/backup -w 32 -b 100`

Backup a container to a path `/mnt/backup` with a derived tarfile name with custom worker count (-w) of 32 and batch size (-b) of 100 files per worker. In this instance, the filename would be in the format `testblobstore-YYY-MM-DD.tar`. Copy the tarfile to the destination container. The path to the container would be `YYYY-MM-DD/testblobstore/mnt/backup/testblobstore-2025-03-18.tar`. 

`./azarchive download-tarfile -t YYY-MM-DD/testblobstore/mnt/backup/testblobstore-YYYY-MM-DD.tar -dp /mnt/backup -w 16`

 Download a tarfile (-t) from the default destination storage container (the destination container in the config file) The name of the tarfile can be found in the Azure console at `home->storage accounts->storage account name->containers`. Navigate the blob finder   until you reach the tarfile. Click on the terfile and it will reveal its full path. This can be pasted into the command line. This command can use a custom worker pool but does not require a batch size.

`/azarchive download-tarfile -t "<fullTarFileContainerPath>" -dc "<connectionStringOfTarFileContainer>" -dn "<tarFileContainerName>" -w 16 -b 200 -dp "<localPathToStoreTarFile>"`

Download a tarfile from a specific storage account and container. 
This might look confusing. The command is using destination connection string and destination container name as a source (-dc -dn). This is because, by default, the destination connection string and destination container name are the _target_ for the backup. In a restore, you want to source the tar file from the target backup location.

`/mnt/app/azarchive restore -t /mnt/backup/testblobstore-YYYY-MM-DD.tar -w 32 -b 100`

Restore a tarfile (-t) including the full path to the default source container (the source being the source storage container defined in the config file) with custom worker count (-w) of 32 and batch size (-b) of 100 files per worker.

`/mnt/app/azarchive count`

Count the number of files in the source container repository (which, during a restore, is the destination container if not set manually). This uses the pager function and is fairly slow. It is, however, the only reliable method of calculating the number of files in a container. There is a value in the Azure console, containers page but it is only updated "periodically".  It should, however be used sparingly as a) it take time to run and b) it consumes credits.

`/mnt/app/azarchive restore -t /mnt/backup/stevetest-2025-03-18.tar -c "DefaultEndpointsProtocol=htt
ps;AccountName=<accountname>;AccountKey=<accouintKey>;EndpointSuffix=core.windows.net" -n <alternativetestblobstore> -w 32 -b 100`

Upload a tarfile to a specific destination using a customer storage account connect string (-c) and customer container name (-n) with custom worker count (-w) of 32 and batch size (-b) of 100 files per worker

***Note*** 
These examples all have real names substituted. Unless you are using the default source and destination from the config file, you should make sure your source and destination accounts and containers are correct. Also note the account connection string must be in double quotes, due to some of the special characters in the connect string

***Note on concurrency vs parallelism with goroutines*** 
In the first example, workers (-w)  is set to 32. This means that 32 goroutines will be launched. These will run _concurrently_, the level of parallelism is defined in this code via a package in the code that calculates the number of cores available in a container.  The reason for having so many processes is that each blob is requested individually, (the write method is different but the behaviour is the same), this is due to a restriction in the Azure API. This means that there is a lot of waits on response which is an ideal time for context switches on the processor. Larger quantities of files will benefit from a larger number, particularly if the file size is small. The reason for this is that the tafile being written to has a MUTEX lock applied and so larger files will queue behind the MUSTEX, which is always cooing to operate serially. On a 8 core container, 32 goroutines is the upper limit where a benefit is seen, after that, performance tails off.  

 The batch size (-b) defines how many filename are pulled from the queue at a time. The queue is fed from a single channel that receives data from a pager API call that returns up to 5000 filenames at a time. The request to this pager has a latency of around 1-2 seconds, The batch size is a balancing act between the number of files in the queue channel, the number of pagess cached to the queue and the number of processes feeding from the queue.  For large numbers of small files, 100 has proven sufficient, in any scenario, a number that is a factor of 5000 to reduce unnecessary pager calls.

 ***Note on throttling*** 
 During testing, it was observed that, under prolonged heavy load, the storage containers run out of credits and requests are throttled. This has an adverse effect on performance and there is no work around for it. Details of throttling can be found on the Azure website.
