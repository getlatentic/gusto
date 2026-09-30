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

variable "manage_github_provider" {
  description = "Whether this stack makes GitHub's identity provider, which the account has one of. It needs the right to make one, which the Terraform user of a limited profile does not have; README.md says how to make it once by hand."
  type        = bool
  default     = false
}

variable "github_subject" {
  description = "How GitHub names the repository in the token it gives a workflow: repo:<owner>/<name>, or with the repository's ids when the repository uses the immutable form. bootstrap.sh reads it from GitHub."
  type        = string
  default     = "repo:getlatentic/haas"
}

variable "orchestrator_instance_type" {
  description = "The type of a demo's front door, which runs no computer."
  type        = string
  default     = "t3.medium"
}

variable "cell_instance_type" {
  description = "The type of a cell, which runs computers: an x86-64 type with nested virtualization, or a .metal type."
  type        = string
  default     = "m8i.xlarge"
}

variable "front_door_instance_type" {
  description = "The type of the front door that stays up. It runs no computer, and a cell is started for those."
  type        = string
  default     = "t3.small"
}
