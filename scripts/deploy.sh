#!/bin/bash

# BMAD Discord Bot Kubernetes Deployment Script
# This script automates the deployment of the BMAD Discord Bot to Kubernetes

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="bmad-bot"
DEPLOYMENT_NAME="bmad-discord-bot"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="${SCRIPT_DIR}/../k8s"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl is not installed or not in PATH"
        exit 1
    fi
    
    # Skip cluster connection check if SKIP_K8S_INTEGRATION is set
    if [ "$SKIP_K8S_INTEGRATION" = "true" ]; then
        print_warning "Skipping Kubernetes cluster connection check (SKIP_K8S_INTEGRATION=true)"
    else
        # Check kubectl cluster connection
        if ! kubectl cluster-info &> /dev/null; then
            print_error "Cannot connect to Kubernetes cluster. Check your kubeconfig."
            exit 1
        fi
    fi
    
    # Check if k8s directory exists
    if [ ! -d "$K8S_DIR" ]; then
        print_error "Kubernetes manifests directory not found: $K8S_DIR"
        exit 1
    fi
    
    print_success "Prerequisites check passed"
}

# Function to create secrets interactively
create_secrets() {
    print_status "Setting up secrets..."
    
    # Check if secrets already exist
    if kubectl get secret bmad-bot-secrets -n "$NAMESPACE" &> /dev/null; then
        print_warning "Secrets already exist. Skipping secret creation."
        print_warning "To update secrets, delete them first: kubectl delete secret bmad-bot-secrets -n $NAMESPACE"
        return 0
    fi
    
    # Get Discord bot token
    if [ -z "$BOT_TOKEN" ]; then
        echo -n "Enter Discord Bot Token: "
        read -s BOT_TOKEN
        echo
    fi
    
    if [ -z "$BOT_TOKEN" ]; then
        print_error "Discord Bot Token is required"
        exit 1
    fi
    
    # Get MySQL password
    if [ -z "$MYSQL_PASSWORD" ]; then
        echo -n "Enter MySQL Password: "
        read -s MYSQL_PASSWORD
        echo
    fi
    
    if [ -z "$MYSQL_PASSWORD" ]; then
        print_error "MySQL Password is required"
        exit 1
    fi
    
    # Create namespace if it doesn't exist
    kubectl apply -f "$K8S_DIR/namespace.yaml"
    
    # Create secret
    kubectl create secret generic bmad-bot-secrets \
        --namespace="$NAMESPACE" \
        --from-literal=BOT_TOKEN="$BOT_TOKEN" \
        --from-literal=MYSQL_PASSWORD="$MYSQL_PASSWORD"
    
    print_success "Secrets created successfully"
}

# Function to deploy using kustomize
deploy_with_kustomize() {
    print_status "Deploying with Kustomize..."
    
    if command -v kustomize &> /dev/null; then
        kustomize build "$K8S_DIR" | kubectl apply -f -
    else
        kubectl apply -k "$K8S_DIR"
    fi
    
    print_success "Deployment applied successfully"
}

# Function to deploy using individual manifests
deploy_with_manifests() {
    print_status "Deploying with individual manifests..."
    
    # Deploy in order
    local manifests=(
        "namespace.yaml"
        "serviceaccount.yaml"
        "configmap.yaml"
        "persistentvolume.yaml"
        "deployment.yaml"
        "networkpolicy.yaml"
        "hpa.yaml"
    )
    
    for manifest in "${manifests[@]}"; do
        if [ -f "$K8S_DIR/$manifest" ]; then
            print_status "Applying $manifest..."
            kubectl apply -f "$K8S_DIR/$manifest"
        else
            print_warning "$manifest not found, skipping..."
        fi
    done
    
    print_success "All manifests applied successfully"
}

# Function to wait for deployment
wait_for_deployment() {
    print_status "Waiting for deployment to be ready..."
    
    if kubectl rollout status deployment/"$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout=300s; then
        print_success "Deployment is ready"
    else
        print_error "Deployment failed or timed out"
        print_status "Checking pod status..."
        kubectl get pods -n "$NAMESPACE"
        print_status "Recent events:"
        kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' | tail -10
        exit 1
    fi
}

# Function to verify deployment
verify_deployment() {
    print_status "Verifying deployment..."
    
    # Check pods
    local pod_status=$(kubectl get pods -n "$NAMESPACE" -l app=bmad-discord-bot -o jsonpath='{.items[0].status.phase}')
    if [ "$pod_status" != "Running" ]; then
        print_error "Pod is not running. Status: $pod_status"
        kubectl describe pods -n "$NAMESPACE" -l app=bmad-discord-bot
        exit 1
    fi
    
    # Check pod logs for startup success
    print_status "Checking application logs..."
    kubectl logs -n "$NAMESPACE" deployment/"$DEPLOYMENT_NAME" --tail=20
    
    print_success "Deployment verification completed"
}

# Function to show deployment status
show_status() {
    print_status "Deployment Status:"
    echo
    kubectl get all -n "$NAMESPACE"
    echo
    print_status "Pod Details:"
    kubectl describe pods -n "$NAMESPACE" -l app=bmad-discord-bot
}

# Function to show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  -k, --kustomize         Use kustomize for deployment (default)"
    echo "  -m, --manifests         Use individual manifests for deployment"
    echo "  -s, --status            Show deployment status only"
    echo "  -v, --verify            Verify existing deployment"
    echo "  --skip-secrets          Skip secret creation (assumes secrets exist)"
    echo "  --dry-run               Show what would be deployed without applying"
    echo
    echo "Environment Variables:"
    echo "  BOT_TOKEN              Discord bot token (will prompt if not set)"
    echo "  MYSQL_PASSWORD         MySQL password (will prompt if not set)"
    echo
    echo "Examples:"
    echo "  $0                     Full deployment with kustomize"
    echo "  $0 -m                  Deploy using individual manifests"
    echo "  $0 -s                  Show current deployment status"
    echo "  BOT_TOKEN=xxx MYSQL_PASSWORD=yyy $0  Deploy with pre-set secrets"
}

# Parse command line arguments
USE_KUSTOMIZE=true
SKIP_SECRETS=false
DRY_RUN=false
STATUS_ONLY=false
VERIFY_ONLY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -k|--kustomize)
            USE_KUSTOMIZE=true
            shift
            ;;
        -m|--manifests)
            USE_KUSTOMIZE=false
            shift
            ;;
        -s|--status)
            STATUS_ONLY=true
            shift
            ;;
        -v|--verify)
            VERIFY_ONLY=true
            shift
            ;;
        --skip-secrets)
            SKIP_SECRETS=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Main execution
print_status "BMAD Discord Bot Kubernetes Deployment"
print_status "======================================"

# Handle status-only mode
if [ "$STATUS_ONLY" = true ]; then
    check_prerequisites
    show_status
    exit 0
fi

# Handle verify-only mode
if [ "$VERIFY_ONLY" = true ]; then
    check_prerequisites
    verify_deployment
    exit 0
fi

# Check prerequisites
check_prerequisites

# Handle dry-run mode
if [ "$DRY_RUN" = true ]; then
    print_status "DRY RUN MODE - No changes will be applied"
    if [ "$USE_KUSTOMIZE" = true ]; then
        print_status "Would deploy with kustomize:"
        if command -v kustomize &> /dev/null; then
            kustomize build "$K8S_DIR"
        else
            kubectl apply -k "$K8S_DIR" --dry-run=client
        fi
    else
        print_status "Would deploy individual manifests"
        kubectl apply -f "$K8S_DIR" --dry-run=client --recursive
    fi
    exit 0
fi

# Create secrets unless skipped
if [ "$SKIP_SECRETS" = false ]; then
    create_secrets
fi

# Deploy based on chosen method
if [ "$USE_KUSTOMIZE" = true ]; then
    deploy_with_kustomize
else
    deploy_with_manifests
fi

# Wait for deployment to be ready
wait_for_deployment

# Verify deployment
verify_deployment

# Show final status
show_status

print_success "BMAD Discord Bot deployment completed successfully!"
print_status "You can monitor the bot with: kubectl logs -f deployment/$DEPLOYMENT_NAME -n $NAMESPACE"