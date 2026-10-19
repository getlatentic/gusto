variable "region" {
  description = "AWS region. It must offer instance_type in one of its own zones."
  type        = string
  default     = "us-east-1"
}

variable "instance_type" {
  description = "An x86-64 type with nested virtualization (c8i, m8i, r8i and their 7th-generation kin) or a .metal type. m8i.2xlarge is 8 vCPU and 32 GiB."
  type        = string
  default     = "m8i.2xlarge"
}

variable "operator_cidrs" {
  description = "Where the E2B API (3000), dashboard (3001) and sandbox proxy (3002) may be reached from. up.sh passes this machine's address; nothing else reaches them."
  type        = list(string)
}

variable "hugepages" {
  description = "2 MiB hugepages the host reserves for sandbox memory. 8192 is 16 GiB, leaving the rest for the host, E2B's services and gusto."
  type        = number
  default     = 8192
}

variable "zone" {
  description = "The Cloudflare zone the POC's name goes in."
  type        = string
  default     = "getlatentic.com"
}

variable "hostname" {
  description = "The POC's name within the zone."
  type        = string
  default     = "poc"
}

variable "budget_usd" {
  description = "The monthly cost at which AWS emails budget_email."
  type        = number
  default     = 100
}

variable "budget_email" {
  description = "Who AWS emails when the month's cost passes 80% of budget_usd, or is forecast to pass it."
  type        = string
}
