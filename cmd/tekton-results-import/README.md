# Tekton Results Import

This program imports Tekton Results data from a PostgreSQL database into KubeArchive by sending CloudEvents for each record.

## Overview

The Tekton Results Import tool:
- Connects to a PostgreSQL database containing Tekton Results data
- Queries all records where `data->>'kind' <> 'Log'`
- Sends a CloudEvent for each record to the KubeArchive sink
- Tracks progress and provides statistics on processed records

## Configuration

### Database Connection

The program reads database connection details from Kubernetes secrets with these environment variables:

- `DATABASE_URL` - PostgreSQL host/URL
- `DATABASE_PORT` - PostgreSQL port (typically 5432)
- `DATABASE_USER` - Database username
- `DATABASE_PASSWORD` - Database password
- `DATABASE_DB` - Database name

### CloudEvents

Each imported record is sent as a CloudEvent with:
- **Source**: `kubearchive.org/tekton-results-import`
- **Type**: `kubearchive.org.tekton-results.import.resource.update` (defined as `constants.TektonResultsImportEventType`)
- **Data**: The JSON data from the database record
- **Extensions**: `apiversion`, `kind`, `name`, `namespace` (extracted from resource metadata)

## Deployment

### 1. Create the Secret

Edit `tekton-results-import-secret.yaml` and update the base64-encoded database credentials:

```bash
# Encode your actual database values
echo -n "your-db-host" | base64
echo -n "5432" | base64
echo -n "your-username" | base64
echo -n "your-password" | base64
echo -n "your-database-name" | base64
```

### 2. Deploy to Kubernetes

Deploy the secret and job:

```bash
# Apply the secret
kubectl apply -f cmd/tekton-results-import/tekton-results-import-secret.yaml

# Deploy the job (uses pre-built quay.io/gallen/tekton-results-import:latest image)
# Note: Job runs with kubearchive-cluster-vacuum service account
kubectl apply -f cmd/tekton-results-import/tekton-results-import-job.yaml
```

### 3. Monitor Progress

```bash
# Follow job logs
kubectl logs -f job/tekton-results-import -n kubearchive

# Check job status
kubectl get job tekton-results-import -n kubearchive
```

## Performance Considerations

- The program is designed to handle millions of records efficiently
- Progress is logged every 1000 records processed
- Database connection uses optimized settings for long-running operations:
  - Single connection pool to avoid connection overhead
  - No statement timeout to allow processing of large datasets
  - Connection timeout limited to 30 seconds for initial connection only
- The job has a 72-hour timeout (`activeDeadlineSeconds: 259200`)
- Resource limits are set to `2Gi` memory and `2` CPU cores

## Output

The program provides:
- Real-time progress logging every 1000 records
- Final statistics showing:
  - Count by Kubernetes resource kind
  - Total number of records processed
- CloudEvent send success/failure details

## Building

This project uses [ko](https://github.com/google/ko) for building container images. All builds should be run from the repository root directory.

### Prerequisites

1. Install ko:
```bash
go install github.com/google/ko@latest
```

2. Set your container registry:
```bash
export KO_DOCKER_REPO=your-registry.com/your-namespace
```

### Build and Push

```bash
# From the repository root directory
cd /path/to/kubearchive

# Build and push image with ko
ko build --base-import-paths ./cmd/tekton-results-import
