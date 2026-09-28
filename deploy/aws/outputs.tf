output "hostname" {
  description = "The POC's name."
  value       = cloudflare_dns_record.poc.name
}

output "address" {
  description = "The instance's fixed public address."
  value       = local.address
}

output "e2b_api_url" {
  value = module.e2b.api_url
}

output "e2b_sandbox_url" {
  value = module.e2b.sandbox_url
}

output "e2b_dashboard_url" {
  value = module.e2b.dashboard_url
}

output "e2b_api_key" {
  value     = module.e2b.e2b_api_key
  sensitive = true
}

output "autoscaling_group" {
  value = module.e2b.autoscaling_group
}
