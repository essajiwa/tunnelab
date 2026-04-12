output "instance_ip" {
  description = "Static external IP address of the TunneLab instance"
  value       = google_compute_address.tunnelab.address
}

output "ssh_command" {
  description = "SSH command to connect to the instance"
  value       = "gcloud compute ssh tunnelab-server --zone=${var.zone} --project=${var.project_id}"
}

output "get_token_command" {
  description = "Retrieve the initial client auth token"
  value       = "gcloud compute ssh tunnelab-server --zone=${var.zone} --project=${var.project_id} --command='sudo cat /var/lib/tunnelab/initial_token.txt'"
}

output "watch_logs_command" {
  description = "Stream TunneLab service logs"
  value       = "gcloud compute ssh tunnelab-server --zone=${var.zone} --project=${var.project_id} --command='sudo journalctl -u tunnelab -f'"
}

output "watch_startup_log_command" {
  description = "Watch the startup script progress (useful right after terraform apply)"
  value       = "gcloud compute ssh tunnelab-server --zone=${var.zone} --project=${var.project_id} --command='sudo tail -f /var/log/tunnelab-startup.log'"
}

output "dns_setup" {
  description = "DNS records to add at your registrar before Let's Encrypt can issue a cert"
  value       = <<-EOT

    ┌─────────────────────────────────────────────────────────────────┐
    │  DNS SETUP REQUIRED                                             │
    │                                                                 │
    │  Add these records at your DNS provider:                        │
    │                                                                 │
    │  Type  Name                   Value                             │
    │  A     ${var.domain}          ${google_compute_address.tunnelab.address}      │
    │  A     *.${var.domain}        ${google_compute_address.tunnelab.address}      │
    │                                                                 │
    │  The wildcard A record is required for subdomain tunnels.       │
    │  Wait for DNS propagation before disabling tls_staging.         │
    └─────────────────────────────────────────────────────────────────┘

    tls_staging is currently: ${var.tls_staging}
    (set tls_staging=false and re-apply once DNS is confirmed)

  EOT
}
