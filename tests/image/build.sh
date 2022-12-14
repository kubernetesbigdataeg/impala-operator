podman build . --tag docker.io/kubernetesbigdataeg/impala:4.1.0-1
podman login docker.io
podman push docker.io/kubernetesbigdataeg/impala:4.1.0-1