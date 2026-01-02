# Simplified example using storage class shorthand
# This example shows the minimal configuration using iso_url
# with the same storage class for both build and output

packer {
  required_plugins {
    kubevirt = {
      source  = "github.com/hashicorp/kubevirt"
      version = ">= 0.9.0"
    }
  }
}

variable "kube_config" {
  type    = string
  default = "${env("KUBECONFIG")}"
}

source "kubevirt-iso" "fedora" {
  # Kubernetes configuration
  kube_config = var.kube_config
  name        = "fedora-42-base"
  namespace   = "default"

  # ISO URL - automatically downloaded and cached by CDI
  iso_url = "https://download.fedoraproject.org/pub/fedora/linux/releases/42/Server/x86_64/iso/Fedora-Server-dvd-x86_64-42-1.1.iso"

  # Storage class shorthand - applies to both build and output
  storage_class = "rook-ceph-block"
  disk_size     = "30Gi"

  # VM configuration
  instance_type = "o1.xlarge"
  preference    = "fedora"
  os_type       = "linux"

  # Network
  networks {
    name = "default"
    pod {}
  }

  # Boot configuration
  media_files = ["./ks.cfg"]
  boot_command = [
    "<wait5>",
    "<up>e",
    "<down><down><end>",
    " inst.ks=hd:LABEL=OEMDRV:/ks.cfg console=ttyS0,115200n8",
    "<leftCtrlOn>x<leftCtrlOff>"
  ]
  boot_wait                 = "5s"
  installation_wait_timeout = "20m"

  # SSH
  communicator     = "ssh"
  ssh_host         = "127.0.0.1"
  ssh_local_port   = 2020
  ssh_remote_port  = 22
  ssh_username     = "user"
  ssh_password     = "root"
  ssh_wait_timeout = "1m"
}

build {
  sources = ["source.kubevirt-iso.fedora"]

  provisioner "shell" {
    inline = [
      "sudo dnf install -y cloud-init",
      "sudo systemctl enable cloud-init"
    ]
  }
}
