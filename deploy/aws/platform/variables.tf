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
  default     = "repo:getlatentic/gusto"
}

variable "orchestrator_instance_type" {
  description = "The type of a demo's front door, which runs no computer."
  type        = string
  default     = "t3.medium"
}

variable "cell_live_instance_type" {
  description = "The type of a cell of the live front door: one with a disk of its own, which holds its sandboxes and is blanked whenever it stops, so a cell can be kept stopped and started again."
  type        = string
  default     = "m8id.xlarge"
}

variable "cell_instance_type" {
  description = "The type of a cell, which runs computers: an x86-64 type with nested virtualization, or a .metal type."
  type        = string
  default     = "m8i.xlarge"
}

variable "front_door_instance_type" {
  description = "The type of the front door that stays up. It runs no computer, and a cell is started for those. It uses about half a gigabyte of memory, and has a swap file for the installs of a start and an upgrade."
  type        = string
  default     = "t3.micro"
}

variable "alert_email" {
  description = "Who is told when spending runs past what the public cloud should cost. No address makes no budget. bootstrap.sh reads ALERT_EMAIL."
  type        = string
  default     = ""
}

variable "monthly_budget_usd" {
  description = "What a month of the public cloud's AWS usage should stay under, before credits. September's was 39.85, 37.10 of it model usage while engines were being measured, and a demo costs cents."
  type        = number
  default     = 60
}

variable "daily_budget_usd" {
  description = "What a day should stay under, before credits. A demo machine left on is 5.30 a day and a measurement run is 2 to 10, so this tells of a runaway the same day."
  type        = number
  default     = 10
}
