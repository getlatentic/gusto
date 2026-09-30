output "launch_template" {
  value = aws_launch_template.host.name
}

output "ci_role" {
  value = aws_iam_role.ci.arn
}

output "host_role" {
  value = aws_iam_role.host.arn
}
