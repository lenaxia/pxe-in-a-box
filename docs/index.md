
# PXE Boot Server

This project provides a Docker image that sets up a PXE Boot server to support both UEFI and BIOS booting. It includes configurations for multiple operating systems (Ubuntu, Talos OS, Debian, Raspbian) and allows for machine-specific configurations using YAML and Jinja2 templates.

## How to Use

### Docker

1. **Build the Docker Image:**
   ```sh
   docker build -t pxe-server .
   ```

2. **Run the Docker Container:**
   ```sh
   docker run -d --name pxe-server -p 69:69/udp -p 80:80 -p 8080:8080 -p 8081:8081 -v <nfs_mount>:/pxe-server/nfs/mnt pxe-server
   ```

### Kubernetes

1. **Create a Persistent Volume Claim (PVC) for NFS:**
   ```yaml
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: pxe-server-pvc
   spec:
     accessModes:
       - ReadWriteMany
     storageClassName: <nfs-storage-class>
     resources:
       requests:
         storage: 10Gi
   ```

2. **Deploy the PXE Server:**
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: pxe-server
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: pxe-server
     template:
       metadata:
         labels:
           app: pxe-server
       spec:
         containers:
         - name: pxe-server
           image: pxe-server:latest
           ports:
           - containerPort: 69
             protocol: UDP
           - containerPort: 80
           - containerPort: 8080
           - containerPort: 8081
           volumeMounts:
           - mountPath: /pxe-server/nfs/mnt
             name: nfs-volume
         volumes:
         - name: nfs-volume
           persistentVolumeClaim:
             claimName: pxe-server-pvc
   ```

3. **Expose the Deployment:**
   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: pxe-server
   spec:
     selector:
       app: pxe-server
     ports:
     - name: tftp
       port: 69
       targetPort: 69
       protocol: UDP
     - name: http
       port: 80
       targetPort: 80
     - name: healthz
       port: 8080
       targetPort: 8080
     - name: metrics
       port: 8081
       targetPort: 8081
     type: LoadBalancer
   ```

## Configuration

### Available OS Templates


- **ubuntu:**
  - Kernel: vmlinuz
  - Initrd: initrd.img
  - Configuration Template: ubuntu.j2
  - Preseed Template: ubuntu_preseed.j2
  - Kickstart Template: N/A
  - Cloud-Init Template: N/A
  - Image URL: http://archive.ubuntu.com/ubuntu/dists/focal/main/installer-amd64/current/legacy-images/netboot/ubuntu-installer/amd64/linux
  - ISO URL: http://releases.ubuntu.com/20.04/ubuntu-20.04.1-live-server-amd64.iso

- **talos_os:**
  - Kernel: vmlinuz
  - Initrd: initrd.img
  - Configuration Template: talos_os.j2
  - Preseed Template: N/A
  - Kickstart Template: talos_kickstart.j2
  - Cloud-Init Template: N/A
  - Image URL: https://releases.talos.dev/v0.12.0/talos-v0.12.0-x86_64-efi.iso
  - ISO URL: https://releases.talos.dev/v0.12.0/talos-v0.12.0-x86_64-efi.iso

- **debian:**
  - Kernel: vmlinuz
  - Initrd: initrd.img
  - Configuration Template: debian.j2
  - Preseed Template: debian_preseed.j2
  - Kickstart Template: N/A
  - Cloud-Init Template: N/A
  - Image URL: http://ftp.debian.org/debian/dists/stable/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux
  - ISO URL: http://ftp.debian.org/debian/dists/stable/main/installer-amd64/current/images/netboot/debian-installer/amd64/initrd.gz

- **raspbian:**
  - Kernel: vmlinuz
  - Initrd: initrd.img
  - Configuration Template: raspbian.j2
  - Preseed Template: N/A
  - Kickstart Template: N/A
  - Cloud-Init Template: cloud_init.j2
  - Image URL: http://downloads.raspberrypi.org/raspios_lite_arm64/images/raspios_lite_arm64-2021-05-28/2021-05-07-raspios-buster-arm64.iso
  - ISO URL: http://downloads.raspberrypi.org/raspios_lite_arm64/images/raspios_lite_arm64-2021-05-28/2021-05-07-raspios-buster-arm64.iso


### Available Machine Templates


- **MAC Address: 00:11:22:33:44:55**
  - OS: ubuntu
  - Static IP: 192.168.1.100
  - Gateway: 192.168.1.1
  - DNS Servers: 8.8.8.8, 8.8.4.4
  - Partitions:
    
    - Disk: /dev/sda
      - Mount Point: /
      - Size: 20G
    
  - SSH Keys:
    
    - ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEA...
    
  - Default User: pxeuser
  - Default Password: pxepassword

- **MAC Address: 66:77:88:99:AA:BB**
  - OS: talos_os
  - Static IP: 192.168.1.101
  - Gateway: 192.168.1.1
  - DNS Servers: 8.8.8.8, 8.8.4.4
  - Partitions:
    
    - Disk: /dev/sdb
      - Mount Point: /
      - Size: 30G
    
  - SSH Keys:
    
    - ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEA...
    
  - Default User: pxeuser
  - Default Password: pxepassword

- **MAC Address: CC:DD:EE:FF:00:11**
  - OS: debian
  - Static IP: 192.168.1.102
  - Gateway: 192.168.1.1
  - DNS Servers: 8.8.8.8, 8.8.4.4
  - Partitions:
    
    - Disk: /dev/sdc
      - Mount Point: /
      - Size: 25G
    
  - SSH Keys:
    
    - ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEA...
    
  - Default User: pxeuser
  - Default Password: pxepassword

- **MAC Address: 22:33:44:55:66:77**
  - OS: raspbian
  - Static IP: 192.168.1.103
  - Gateway: 192.168.1.1
  - DNS Servers: 8.8.8.8, 8.8.4.4
  - Partitions:
    
    - Disk: /dev/sdd
      - Mount Point: /
      - Size: 15G
    
  - SSH Keys:
    
    - ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEA...
    
  - Default User: pxeuser
  - Default Password: pxepassword
