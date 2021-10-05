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
provider "openstack" {
}

# Network configuration

data "openstack_networking_network_v2" "public" {
  name = "public"
}

# Create router for connection public and anyflashcards_private networks
resource "openstack_networking_router_v2" "anyflashcards_router" {
  name                = "anyflashcards_router"
  admin_state_up      = true
  external_network_id = "${data.openstack_networking_network_v2.public.id}"
}

# Create anyflashcards private network
resource "openstack_networking_network_v2" "anyflashcards_network" {
  name           = "anyflashcards_network"
  admin_state_up = "true"
}

# Create anyflashcards private sub netwok
resource "openstack_networking_subnet_v2" "anyflashcards_subnet_1" {
  name       = "anyflashcards_subnet_1"
  network_id = "${openstack_networking_network_v2.anyflashcards_network.id}"
  cidr       = "192.168.1.0/24"
  ip_version = 4
}

# Connect public and anyflashcards_private networks
resource "openstack_networking_router_interface_v2" "router_interface_1" {
  router_id = "${openstack_networking_router_v2.anyflashcards_router.id}"
  subnet_id = "${openstack_networking_subnet_v2.anyflashcards_subnet_1.id}"
}

# Create anyflashcards security group
resource "openstack_compute_secgroup_v2" "anyflashcards_secgroup" {
  name        = "anyflashcards_secgroup"
  description = "Security group for anyflashcards minikube server"
}
# Create port in anyflashcards network for anyflashcards minikube server
resource "openstack_networking_port_v2" "anyflascards_network_port" {
  name               = "anyflascards_network_port"
  network_id         = "${openstack_networking_network_v2.anyflashcards_network.id}"
  admin_state_up     = "true"
  security_group_ids = ["${openstack_compute_secgroup_v2.anyflashcards_secgroup.id}"]

  fixed_ip {
    subnet_id  = "${openstack_networking_subnet_v2.anyflashcards_subnet_1.id}"
    ip_address = "192.168.1.10"
  }
}

# Create keypair for minikube server
resource "openstack_compute_keypair_v2" "anyflashcards-keypair" {
  name       = "anyflashcards-keypair"
  public_key = file("anyflashcards_rsa.pub")
}

# Create volume for minikube server
resource "openstack_blockstorage_volume_v2" "anyflashcards_volume" {
  name = "anyflashcards_volume"
  size = 20
}

# Create minikube server instance
resource "openstack_compute_instance_v2" "anyflashcards-minikube" {
  name            = "anyflashcards-minikube"
  image_name      = "test-bionic-server-cloudimg-amd64-20210426"
  flavor_name     = "kaas.small"
  key_pair = "anyflashcards-keypair"
  config_drive    = true
  user_data       = file("cloud_init.yaml")
  network {
    port = "${openstack_networking_port_v2.anyflascards_network_port.id}"
  }
}

# Attach volube to minikube server
resource "openstack_compute_volume_attach_v2" "attached" {
  instance_id = "${openstack_compute_instance_v2.anyflashcards-minikube.id}"
  volume_id   = "${openstack_blockstorage_volume_v2.anyflashcards_volume.id}"
}

# TODO Add resource imege
# TODO Add resource flavor
# TODO Move frome using of cloud_init to Ansible playbook