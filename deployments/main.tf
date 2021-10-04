# Define required providers
terraform {
  required_version = ">= 0.14.0"
  required_providers {
    openstack = {
      source  = "terraform-provider-openstack/openstack"
      version = "~> 1.35.0"
    }
  }
}

# Configure the OpenStack Provider
# openrc.sh is used for configuration

module "kubernetes" {
  source  = "ptsgr/kubernetes/openstack"
  version = "0.1.1"
  cluster_name = "anyflashcards"
  master_flavor_id = 237b938d-670d-434b-b6c7-70e8a84a13ca
  worker_flavor_id = 237b938d-670d-434b-b6c7-70e8a84a13ca
  vms_image_id =
  
}

