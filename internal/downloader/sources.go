// Package downloader handles fetching OS boot assets (kernels, initramfs)
// from upstream sources and verifying their integrity.
//
// Sources are resolved per-OS-type:
//   - Talos official releases: GitHub release artifacts + SHA256 checksums
//   - Talos Image Factory: factory.talos.dev artifacts
//   - Ubuntu/Debian/Arch: configurable URLs (operators may need to adjust)
package downloader

import (
	"fmt"

	"github.com/homelab/pxe-in-a-box/internal/config"
)

// FileSpec describes a single file to download: its remote URL,
// the filename to save as in the asset directory, and an optional
// expected SHA256 hash for verification.
type FileSpec struct {
	URL      string // Full remote URL
	Filename string // Destination filename (e.g., "vmlinuz")
	SHA256   string // Expected SHA256 hex (empty = skip verification)
}

// AssetSpec describes all files needed for a single asset,
// where to store them, and where to find checksums.
type AssetSpec struct {
	ID          string
	OSType      config.OSType
	Arch        string
	BaseDir     string // e.g., "/assets/talos-v1.10.6/amd64"
	Files       []FileSpec
	ChecksumURL string // URL to a sha256sum.txt file (optional)
}

// ResolveAssetSpecs converts the config's asset definitions into concrete
// download specifications with full URLs and filenames.
func ResolveAssetSpecs(cfg *config.AssetsConfig) []AssetSpec {
	var specs []AssetSpec

	for _, a := range cfg.Talos {
		specs = append(specs, resolveTalos(a))
	}
	for _, a := range cfg.Ubuntu {
		specs = append(specs, resolveUbuntu(a))
	}
	for _, a := range cfg.Debian {
		specs = append(specs, resolveDebian(a))
	}
	for _, a := range cfg.Arch {
		specs = append(specs, resolveArch(a))
	}

	return specs
}

func resolveTalos(a config.TalosAsset) AssetSpec {
	baseDir := fmt.Sprintf("/assets/%s/%s", a.ID, a.Arch)
	spec := AssetSpec{
		ID:      a.ID,
		OSType:  config.OSTypeTalos,
		Arch:    a.Arch,
		BaseDir: baseDir,
	}

	if a.ImageFactoryHash != "" {
		base := fmt.Sprintf("https://factory.talos.dev/image/%s/%s", a.ImageFactoryHash, a.Version)
		spec.Files = []FileSpec{
			{URL: base + "/kernel-amd64", Filename: "vmlinuz"},
			{URL: base + "/initramfs-amd64.xz", Filename: "initramfs.xz"},
		}
		if a.DownloadUKI {
			spec.Files = append(spec.Files, FileSpec{
				URL:      base + "/metal-amd64-uki.efi",
				Filename: "metal-amd64-uki.efi",
			})
		}
	} else {
		base := fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s", a.Version)
		spec.Files = []FileSpec{
			{URL: base + "/vmlinuz-amd64", Filename: "vmlinuz"},
			{URL: base + "/initramfs-amd64.xz", Filename: "initramfs.xz"},
		}
		if a.DownloadUKI {
			spec.Files = append(spec.Files, FileSpec{
				URL:      base + "/metal-amd64-uki.efi",
				Filename: "metal-amd64-uki.efi",
			})
		}
		spec.ChecksumURL = base + "/sha256sum.txt"
	}

	if len(a.SHA256) > 0 {
		for i := range spec.Files {
			if hash, ok := a.SHA256[spec.Files[i].Filename]; ok {
				spec.Files[i].SHA256 = hash
			}
		}
	}

	return spec
}

func resolveUbuntu(a config.UbuntuAsset) AssetSpec {
	baseDir := fmt.Sprintf("/assets/%s/%s", a.ID, a.Arch)
	// Ubuntu 24.04+ dropped traditional d-i netboot. The URLs below
	// work for releases that have the netboot directory. Operators may
	// need to manually stage files for newer releases.
	base := fmt.Sprintf("http://archive.ubuntu.com/ubuntu/dists/%s/main/installer-%s/current/legacy-images/netboot/ubuntu-installer/%s", a.Release, a.Arch, a.Arch)

	return AssetSpec{
		ID:      a.ID,
		OSType:  config.OSTypeUbuntu,
		Arch:    a.Arch,
		BaseDir: baseDir,
		Files: []FileSpec{
			{URL: base + "/linux", Filename: "linux"},
			{URL: base + "/initrd", Filename: "initrd"},
		},
	}
}

func resolveDebian(a config.DebianAsset) AssetSpec {
	baseDir := fmt.Sprintf("/assets/%s/%s", a.ID, a.Arch)
	base := fmt.Sprintf("https://deb.debian.org/debian/dists/%s/main/installer-%s/current/images/netboot/debian-installer/%s", a.Release, a.Arch, a.Arch)

	return AssetSpec{
		ID:      a.ID,
		OSType:  config.OSTypeDebian,
		Arch:    a.Arch,
		BaseDir: baseDir,
		Files: []FileSpec{
			{URL: base + "/linux", Filename: "linux"},
			{URL: base + "/initrd.gz", Filename: "initrd.gz"},
		},
	}
}

func resolveArch(a config.ArchAsset) AssetSpec {
	baseDir := fmt.Sprintf("/assets/%s/%s", a.ID, a.Arch)
	base := "https://mirror.archlinux.org/iso/latest"

	return AssetSpec{
		ID:      a.ID,
		OSType:  config.OSTypeArch,
		Arch:    a.Arch,
		BaseDir: baseDir,
		Files: []FileSpec{
			{URL: base + "/boot/x86_64/vmlinuz-linux", Filename: "vmlinuz-linux"},
			{URL: base + "/boot/x86_64/initramfs-linux.img", Filename: "initramfs-linux.img"},
		},
	}
}
