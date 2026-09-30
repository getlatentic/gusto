variable "region" {
  description = "AWS region. It must offer instance_type in one of its own zones."
  type        = string
  default     = "us-east-1"
}

variable "bucket" {
  description = "The bucket demo-store.sh makes, which holds the release bundles, the runner image and the certificate."
  type        = string
}

variable "instance_type" {
  description = "An x86-64 type with nested virtualization, or a .metal type."
  type        = string
  default     = "m8i.2xlarge"
}

variable "repository" {
  description = "The GitHub repository whose main branch may launch and remove demo hosts."
  type        = string
  default     = "getlatentic/haas"
}
