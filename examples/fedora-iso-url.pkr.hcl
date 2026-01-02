# Example Packer template demonstrating the new ISO URL and storage class features
# for the KubeVirt ISO builder.
#
# This template showcases:
# 1. Automatic ISO download via iso_url (no pre-provisioning required)
# 2. Separate storage classes for ISO, build, and output volumes
# 3. Building on fast local storage and outputting to persistent shared storage

packer {
  required_plugins {
    kubevirt = {
      source  = "github.com/hashicorp/kubevirt"
      version = ">= 0.9.0"  # Requires version with iso_url support
    }
  }
}

variable "kube_config" {
  type    = string
  default = "${env("KUBECONFIG")}"
}

variable "namespace" {
  type    = string
  default = "actions-runner-system"
  description = "Kubernetes namespace for build resources"
}

variable "output_name" {
  type    = string
  default = "fedora-runner-base"
  description = "Name for the final output DataVolume"
}

locals {
  # Unique name for the temporary build VM
  build_vm_name = "fedora-build-${formatdate("YYYYMMDDhhmm", timestamp())}"
}

source "kubevirt-iso" "fedora" {
  # Kubernetes configuration
  kube_config = var.kube_config
  namespace   = var.namespace

  # ====================================================================
  # NEW FEATURE: ISO URL Source
  # ====================================================================
  # Automatically download and provision the ISO instead of requiring
  # a pre-created DataVolume. CDI will handle the download via HTTP.
  iso_url = "https://download.fedoraproject.org/pub/fedora/linux/releases/42/Server/x86_64/iso/Fedora-Server-dvd-x86_64-42-1.1.iso"

  # ISO will be cached by default in a DataVolume named: packer-iso-<hash>
  # To delete after build, set: delete_iso = true
  # To override the auto-generated name, set: iso_name = "my-custom-iso-name"

  # ====================================================================
  # NEW FEATURE: ISO Storage Class
  # ====================================================================
  # Use a RWX-capable storage class for the ISO so multiple builds
  # can share the cached ISO simultaneously
  iso_storage_class = "rook-cephfs"  # RWX for parallel builds

  # ====================================================================
  # NEW FEATURE: Build and Output Storage Classes
  # ====================================================================
  # Build on fast local NVMe storage for better performance
  build_storage_class = "local-path"  # Fast local storage
  disk_size           = "30Gi"

  # Output to persistent Ceph storage for durability and distribution
  output_storage_class = "rook-ceph-block"  # Persistent shared storage
  name                 = local.build_vm_name  # Temporary build VM name

  # NOTE: The build DataVolume will be automatically cloned to the output
  # storage class after provisioning completes. The temporary build volume
  # will then be cleaned up.

  # VM configuration
  instance_type = "o1.xlarge"
  preference    = "fedora"
  os_type       = "linux"

  # Network configuration
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
  installation_wait_timeout = "10m"

  # SSH configuration
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

  # Expand root partition
  provisioner "shell" {
    inline = [
      "echo 'Expanding root partition to use full disk...'",
      "sudo dnf install -y cloud-utils-growpart",
      "sudo growpart /dev/vda 3",
      "sudo xfs_growfs /",
      "df -h /",
      "echo 'Root partition expanded successfully'"
    ]
  }

  # Configure serial console
  provisioner "shell" {
    inline = [
      "echo 'Configuring serial console for KubeVirt...'",
      "sudo tee -a /etc/default/grub > /dev/null << 'EOF'",
      "GRUB_TERMINAL=\"serial console\"",
      "GRUB_SERIAL_COMMAND=\"serial --speed=115200 --unit=0 --word=8 --parity=no --stop=1\"",
      "EOF",
      "sudo sed -i 's/^GRUB_CMDLINE_LINUX=\"/GRUB_CMDLINE_LINUX=\"console=tty0 console=ttyS0,115200n8 /' /etc/default/grub",
      "sudo grub2-mkconfig -o /boot/grub2/grub.cfg",
      "echo 'Serial console configured'"
    ]
  }

  # Install cloud-init
  provisioner "shell" {
    inline = [
      "echo 'Installing cloud-init...'",
      "sudo dnf install -y cloud-init",
      "sudo systemctl enable cloud-init",
      "echo 'Cloud-init installed and enabled'"
    ]
  }

  # Clean up
  provisioner "shell" {
    inline = [
      "echo 'Cleaning up...'",
      "sudo dnf clean all",
      "echo 'Cleanup complete'"
    ]
  }
}
