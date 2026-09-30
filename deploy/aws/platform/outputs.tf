output "launch_template" {
  value = aws_launch_template.host.name
}

output "ci_role" {
  value = aws_iam_role.ci.arn
}

output "host_role" {
  value = aws_iam_role.host.arn
}

output "launch_templates" {
  value = { for role, template in aws_launch_template.role : role => template.name }
}

output "file_system" {
  description = "Its id is the repository variable HAAS_EFS."
  value       = aws_efs_file_system.shared.id
}
