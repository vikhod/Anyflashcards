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

### Network configuration ###
data "openstack_networking_network_v2" "public" {
  name = "public"
}

resource "openstack_networking_router_v2" "anyflashcards_router" {
  name                = "anyflashcards_router"
  admin_state_up      = true
  external_network_id = "${data.openstack_networking_network_v2.public.id}"
}

resource "openstack_networking_network_v2" "anyflashcards_network" {
  name           = "anyflashcards_network"
  admin_state_up = true
}

resource "openstack_networking_subnet_v2" "anyflashcards_subnet_1" {
  name       = "anyflashcards_subnet_1"
  network_id = "${openstack_networking_network_v2.anyflashcards_network.id}"
  cidr       = "192.168.1.0/24"
  ip_version = 4
}

resource "openstack_networking_router_interface_v2" "router_interface_1" {
  router_id = "${openstack_networking_router_v2.anyflashcards_router.id}"
  subnet_id = "${openstack_networking_subnet_v2.anyflashcards_subnet_1.id}"
}

resource "openstack_compute_secgroup_v2" "anyflashcards_secgroup" {
  name        = "anyflashcards_secgroup"
  description = "Security group for anyflashcards minikube server"
}

resource "openstack_networking_secgroup_rule_v2" "allow_icmp" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "icmp"
  remote_ip_prefix  = "0.0.0.0/0"
  security_group_id = "${openstack_compute_secgroup_v2.anyflashcards_secgroup.id}"
}

resource "openstack_networking_secgroup_rule_v2" "allow_ssh" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "tcp"
  port_range_min    = 22
  port_range_max    = 22
  remote_ip_prefix  = "0.0.0.0/0"
  security_group_id = "${openstack_compute_secgroup_v2.anyflashcards_secgroup.id}"
}

resource "openstack_networking_secgroup_rule_v2" "allow_http" {
  direction         = "ingress"
  ethertype         = "IPv4"
  protocol          = "tcp"
  port_range_min    = 1
  port_range_max    = 65535
  remote_ip_prefix  = "0.0.0.0/0"
  security_group_id = "${openstack_compute_secgroup_v2.anyflashcards_secgroup.id}"
}

resource "openstack_networking_port_v2" "anyflascards_network_port" {
  name               = "anyflascards_network_port"
  network_id         = "${openstack_networking_network_v2.anyflashcards_network.id}"
  admin_state_up     = true
  security_group_ids = ["${openstack_compute_secgroup_v2.anyflashcards_secgroup.id}"]

  fixed_ip {
    subnet_id  = "${openstack_networking_subnet_v2.anyflashcards_subnet_1.id}"
    ip_address = "192.168.1.10"
  }
}

resource "openstack_networking_floatingip_v2" "anyflashcards_extip" {
  pool = "public"
}

### Minikube server configuration
resource "openstack_compute_keypair_v2" "anyflashcards-keypair" {
  name       = "anyflashcards-keypair"
  public_key = file("anyflashcards_rsa.pub")
}

resource "openstack_blockstorage_volume_v2" "anyflashcards_volume" {
  name = "anyflashcards_volume"
  size = 20
}

resource "openstack_compute_instance_v2" "vh-af-minikube" {
  name            = "vh-af-minikube"
  image_name      = "focal-server-cloudimg-amd64-20211006"
  flavor_name     = "kaas.small"
  key_pair = "anyflashcards-keypair"
 
  network {
    port = "${openstack_networking_port_v2.anyflascards_network_port.id}"
  }
}

resource "openstack_compute_floatingip_associate_v2" "connected" {
  floating_ip = "${openstack_networking_floatingip_v2.anyflashcards_extip.address}"
  instance_id = "${openstack_compute_instance_v2.vh-af-minikube.id}"
}

resource "openstack_compute_volume_attach_v2" "attached" {
  instance_id = "${openstack_compute_instance_v2.vh-af-minikube.id}"
  volume_id   = "${openstack_blockstorage_volume_v2.anyflashcards_volume.id}"
}

resource "null_resource" "ansibled" {
  depends_on = [
    openstack_compute_instance_v2.vh-af-minikube,
    openstack_compute_floatingip_associate_v2.connected,
    openstack_compute_volume_attach_v2.attached
  ]

  provisioner "local-exec" {
    command = <<EOD
cat <<EOF > anyflashcards_hosts 
[minikube] 
${openstack_networking_floatingip_v2.anyflashcards_extip.address}

[minikube:vars]
ansible_ssh_user=ubuntu
ansible_ssh_private_key_file=anyflashcards_rsa
EOF
EOD
  }

  provisioner "local-exec" {
    command = "ansible-playbook -i anyflashcards_hosts minikube.yml"
  }
  
}

# TODO Move frome using of cloud_init to Ansible playbook