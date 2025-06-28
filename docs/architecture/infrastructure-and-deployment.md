# Infrastructure and Deployment

## Infrastructure as Code

A `Dockerfile` will be the primary IaC artifact, defining the application's runtime environment. For local development, a `docker-compose.yml` file can be added to simplify running the container and managing environment variables.

## Deployment Strategy

The application will be deployed as a single Docker container. This container can be run on any cloud provider's container service, such as Google Cloud Run, AWS ECS, or DigitalOcean App Platform. The strategy is to simply stop the old container and start the new one on deployment.

## Environments

  * **Local**: Developers run the container on their local machine using Docker Desktop.
  * **Production**: A single container running on a cloud provider.

## Rollback Strategy

Rollback is achieved by re-deploying the previously known-good Docker image tag.