test
# pxe-boot
A self contained, configurable PXE boot image 

/pxe-server
├── /tftpboot
│   ├── /bios
│   │   └── pxelinux.0
│   ├── /uefi
│   │   └── grubx64.efi
│   └── /pxelinux.cfg
│       └── default
├── /http
│   ├── /images
│   │   └── <os-name>.iso
│   ├── /configs
│   │   └── <os-name>.cfg
│   ├── /machine-specific
│   │   └── <mac-address>.yaml
│   ├── /preseed
│   │   └── <os-name>.preseed
│   ├── /cloud-init
│   │   └── <os-name>.yaml
│   └── /kickstart
│       └── <os-name>.ks
├── /scripts
│   ├── download_images.py
│   ├── apply_configurations.py
│   ├── pxe_menu.py
│   ├── healthz.py
│   └── metrics.py
├── /templates
│   ├── os_templates.yaml
│   ├── machine_templates.yaml
│   ├── default_pxe_menu.j2
│   ├── ubuntu.j2
│   ├── talos_os.j2
│   ├── debian.j2
│   ├── raspbian.j2
│   ├── ubuntu_preseed.j2
│   ├── debian_preseed.j2
│   ├── talos_kickstart.j2
│   └── cloud_init.j2
└── /nfs
    └── /mnt
