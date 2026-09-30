variable "region" {
  description = "AWS region. It must offer instance_type in one of its own zones."
  type        = string
  default     = "us-east-1"
}

variable "instance_type" {
  description = "An x86-64 type with nested virtualization (c8i, m8i, r8i and their 7th-generation kin) or a .metal type. The test needs 4 vCPU and 16 GiB to run a sandbox and a gateway."
  type        = string
  default     = "m8i.xlarge"
}

variable "operator_cidrs" {
  description = "Where SSH may be reached from. up.sh passes this machine's address; nothing else reaches the instance."
  type        = list(string)
}

variable "public_key" {
  description = "The SSH public key the instance accepts. up.sh makes a key pair for it under .state/, and down.sh removes it."
  type        = string
}

variable "name" {
  description = "What the instance, its group and its key are called."
  type        = string
  default     = "haas-poc-openshell"
}

variable "public_https" {
  description = "Open port 443 to everyone, for a host that serves the public cloud's pages. SSH stays open to operator_cidrs only."
  type        = bool
  default     = false
}

variable "demo_bucket" {
  description = "A bucket the instance may read and write, which holds what outlives it: the certificate and the runner image. demo-store.sh makes it, and this stack does not manage it, so down.sh leaves it."
  type        = string
  default     = ""
}
