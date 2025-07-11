#!/bin/bash

# Script de configuración para Raspberry Pi - Diplo LXC
# Uso: sudo ./raspberry_pi_setup.sh

set -e

echo "=== Configurando Diplo LXC en Raspberry Pi ==="

# Verificar que se ejecuta como root
if [[ $EUID -ne 0 ]]; then
   echo "Este script debe ejecutarse como root (sudo)"
   exit 1
fi

# Actualizar sistema
echo "Actualizando sistema..."
apt-get update && apt-get upgrade -y

# Instalar LXC y dependencias
echo "Instalando LXC..."
apt-get install -y lxc lxc-templates bridge-utils

# Instalar Go (si no está instalado)
if ! command -v go &> /dev/null; then
    echo "Instalando Go..."
    wget https://go.dev/dl/go1.21.5.linux-arm64.tar.gz
    tar -C /usr/local -xzf go1.21.5.linux-arm64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    source /etc/profile
fi

# Configurar bridge de red para LXC
echo "Configurando red LXC..."
cat > /etc/lxc/default.conf << EOF
lxc.network.type = veth
lxc.network.link = lxcbr0
lxc.network.flags = up
lxc.network.hwaddr = 00:16:3e:xx:xx:xx
EOF

# Habilitar forwarding de IP
echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf
sysctl -p

# Crear directorio para templates personalizados
mkdir -p /usr/share/lxc/templates/diplo

# Configurar cgroups para Raspberry Pi
echo "Configurando cgroups..."
sed -i 's/$/ cgroup_enable=cpuset cgroup_enable=memory cgroup_memory=1/' /boot/cmdline.txt

# Instalar herramientas adicionales
echo "Instalando herramientas adicionales..."
apt-get install -y git curl wget htop

# Crear usuario diplo
echo "Creando usuario diplo..."
useradd -m -s /bin/bash diplo
usermod -aG lxd diplo

# Crear directorio de trabajo
mkdir -p /opt/diplo
chown diplo:diplo /opt/diplo

# Crear servicio systemd
cat > /etc/systemd/system/diplo.service << EOF
[Unit]
Description=Diplo LXC Platform
After=network.target

[Service]
Type=simple
User=diplo
WorkingDirectory=/opt/diplo
ExecStart=/opt/diplo/diplo
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Habilitar servicio
systemctl daemon-reload
systemctl enable diplo

echo "=== Configuración completada ==="
echo "Pasos siguientes:"
echo "1. Reiniciar el Raspberry Pi: sudo reboot"
echo "2. Compilar Diplo: cd /opt/diplo && go build -o diplo ./cmd/diplo"
echo "3. Iniciar servicio: sudo systemctl start diplo"
echo "4. Verificar estado: sudo systemctl status diplo"
echo ""
echo "Diplo estará disponible en: http://$(hostname -I | cut -d' ' -f1):8080" 