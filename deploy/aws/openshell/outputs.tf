output "address" {
  value = aws_instance.spike.public_ip
}

output "instance_id" {
  value = aws_instance.spike.id
}

output "user" {
  value = "ubuntu"
}
