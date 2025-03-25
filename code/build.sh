echo "$(DATE) Building arm64 version"
go build -o run .
if [[ $? -ne 0 ]]; then
	exit 1
fi

echo "$(DATE) Building amd64 version"
GOOS=linux GOARCH=amd64 go build -o run_amd64 .
if [[ $? -ne 0 ]]; then
	exit 1
fi

echo "$(DATE) uploading amd64 version to container"
kubectl cp ./run_amd64 azure-archive:/mnt/app/azarchive
if [[ $? -ne 0 ]]; then
        exit 1
fi
