package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_HCL_TerraformResources(t *testing.T) {
	src := `terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "env" {
  type = string
}

resource "aws_s3_bucket" "artifacts" {
  bucket = "${var.env}-artifacts"
  tags = {
    Environment = var.env
  }
}

resource "aws_s3_bucket" "logs" {
  bucket = "${var.env}-logs"
}

output "bucket_arn" {
  value = aws_s3_bucket.artifacts.arn
}
`
	fm, ok := filemap.Generate(src, "main.tf")
	if !ok {
		t.Fatal("expected deterministic map for Terraform file")
	}

	if !strings.Contains(fm.Summary, "Terraform") {
		t.Errorf("expected Terraform summary, got: %s", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "resource") {
		t.Errorf("expected resource count in summary, got: %s", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "variable") {
		t.Errorf("expected variable count in summary, got: %s", fm.Summary)
	}

	// Verify all block types appear as symbols.
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["terraform"] {
		t.Errorf("expected 'terraform' symbol in %v", fm.Symbols)
	}
	if !symbolSet["provider aws"] {
		t.Errorf("expected 'provider aws' symbol in %v", fm.Symbols)
	}
	if !symbolSet["variable region"] {
		t.Errorf("expected 'variable region' symbol in %v", fm.Symbols)
	}
	if !symbolSet["resource aws_s3_bucket.artifacts"] {
		t.Errorf("expected 'resource aws_s3_bucket.artifacts' symbol in %v", fm.Symbols)
	}
	if !symbolSet["output bucket_arn"] {
		t.Errorf("expected 'output bucket_arn' symbol in %v", fm.Symbols)
	}
}

func TestGenerate_HCL_DataSources(t *testing.T) {
	src := `data "aws_vpc" "selected" {
  tags = {
    Name = "main"
  }
}

data "aws_subnets" "private" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.selected.id]
  }
}

resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"
  subnet_id     = data.aws_subnets.private.ids[0]
}
`
	fm, ok := filemap.Generate(src, "compute.tf")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["data aws_vpc.selected"] {
		t.Errorf("expected 'data aws_vpc.selected' symbol in %v", fm.Symbols)
	}
	if !symbolSet["data aws_subnets.private"] {
		t.Errorf("expected 'data aws_subnets.private' symbol in %v", fm.Symbols)
	}
	if !symbolSet["resource aws_instance.web"] {
		t.Errorf("expected 'resource aws_instance.web' symbol in %v", fm.Symbols)
	}
}

func TestGenerate_HCL_LineRangesNoOverlap(t *testing.T) {
	src := `variable "a" {
  type = string
}

variable "b" {
  type = number
}

variable "c" {
  type = bool
}
`
	fm, ok := filemap.Generate(src, "vars.tf")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("entry has empty Lines: %+v", e)
		}
	}
	// First variable starts on line 1.
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("first variable should start on line 1, got: %s", fm.Map[0].Lines)
	}
}

func TestGenerate_HCL_ModuleBlocks(t *testing.T) {
	src := `module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
  name    = "main"
  cidr    = "10.0.0.0/16"
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "20.0.0"
  cluster_name = "prod"
}
`
	fm, ok := filemap.Generate(src, "modules.tf")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Summary, "module") {
		t.Errorf("expected module in summary, got: %s", fm.Summary)
	}
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["module vpc"] {
		t.Errorf("expected 'module vpc' in %v", fm.Symbols)
	}
	if !symbolSet["module eks"] {
		t.Errorf("expected 'module eks' in %v", fm.Symbols)
	}
}

func TestGenerate_HCL_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "main.tf")
	if ok {
		t.Error("expected fallthrough for empty .tf file")
	}
}

func TestGenerate_HCL_FallsThrough_NoBlocks(t *testing.T) {
	// Comments and locals-only content without block structure.
	src := "# just a comment\n# nothing here\n"
	_, ok := filemap.Generate(src, "empty.tf")
	if ok {
		t.Error("expected fallthrough for .tf file with no blocks")
	}
}

func TestGenerate_HCL_TerraformLockFallsThrough(t *testing.T) {
	// .terraform.lock.hcl is a provider version lock; must never be digested.
	src := `provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.31.0"
  constraints = "~> 5.0"
  hashes = [
    "h1:abc123",
  ]
}
`
	_, ok := filemap.Generate(src, ".terraform.lock.hcl")
	if ok {
		t.Error(".terraform.lock.hcl must fall through — provider pins must reach the model verbatim")
	}
}

func TestGenerate_HCL_GeneratedFile(t *testing.T) {
	src := `# This file is maintained automatically by "terraform init".
# Manual edits may be lost in future updates.

provider "registry.terraform.io/hashicorp/random" {
  version = "3.6.0"
}
`
	fm, ok := filemap.Generate(src, "providers.tf")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary for auto-generated file, got: %s", fm.Summary)
	}
}

func TestGenerate_HCL_DotHCLExtension(t *testing.T) {
	src := `variable "cluster_name" {
  type = string
}
`
	fm, ok := filemap.Generate(src, "config.hcl")
	if !ok {
		t.Fatal("expected deterministic map for .hcl extension")
	}
	if !strings.Contains(fm.Summary, "HCL") {
		t.Errorf("expected HCL in summary for .hcl file, got: %s", fm.Summary)
	}
}
