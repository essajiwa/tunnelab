variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "asia-southeast2"
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "asia-southeast2-a"
}

variable "domain" {
  description = "Public domain for the TunneLab server (e.g. id.tunnelab.dev)"
  type        = string
  default     = "id.tunnelab.dev"
}

variable "tls_email" {
  description = "Email for Let's Encrypt certificate notifications"
  type        = string
  default     = "me@essa.jp.net"
}

variable "tls_staging" {
  description = "Use Let's Encrypt staging (true for testing DNS, false for production)"
  type        = bool
  default     = true
}

variable "machine_type" {
  description = "GCP Compute Engine machine type"
  type        = string
  default     = "e2-micro"
}

variable "disk_size_gb" {
  description = "Boot disk size in GB"
  type        = number
  default     = 20
}

variable "repo_url" {
  description = "Git repository URL for TunneLab source code"
  type        = string
  default     = "https://github.com/essajiwa/tunnelab.git"
}

variable "tcp_port_range" {
  description = "Port range for TCP/UDP tunnels (e.g. 50000-50010)"
  type        = string
  default     = "50000-50010"
}

variable "ssh_allowed_ips" {
  description = "CIDR ranges allowed to SSH to the instance. Restrict in production."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}
