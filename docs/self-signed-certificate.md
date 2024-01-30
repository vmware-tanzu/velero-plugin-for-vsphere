# Self-signed certificate

Velero starts to support installing default BackupStorageLocation with self-signed certificate from v1.4.2. More details please refer to https://velero.io/docs/v1.7/self-signed-certificates/.

To keep consistency with velero, Velero plugin for vsphere also supports using velero-plugin-for-vsphere with a storage provider secured by a self-signed certificate.

* Vanilla cluster: supported since v1.1.1
* Guest cluster: supported since v1.1.1
* Supervisor cluster: not supported

To install with a storage provider secured by a self-signed certificate, the --cacert option needs to be added to provide a path to a certificate bundle to trust. This certificate is used to verify TLS connection to the object store when backing up and restoring. The object store is specified with ` --backup-location-config` in the install command.

Here is an example install command:
```text
BUCKET=velero-minio
REGION=minio
S3URL=<s3url>
CACERT=./certs/public.crt
velero install --provider aws \
                --bucket $BUCKET \
                --secret-file ./credentials-velero \
                --plugins velero/velero-plugin-for-aws:v1.0.0 \
                --snapshot-location-config region=$REGION \
                --backup-location-config region=$REGION,s3ForcePathStyle="true",s3Url=$S3URL \
                --cacert $CACERT
```