# Configuración de Containerd en Raspberry Pi

Este documento explica cómo configurar containerd en Raspberry Pi para usar con Diplo.

## ¿Por qué Containerd en Raspberry Pi?

- **Mejor rendimiento**: Containerd es más ligero que Docker
- **Menor uso de memoria**: Ideal para dispositivos con recursos limitados
- **Mejor soporte ARM**: Optimizado para arquitectura ARM
- **Más rápido**: Inicio y operaciones más rápidas

## Instalación Automática

### Opción 1: Usando la API de Diplo

```bash
# Instalar containerd automáticamente
curl -X POST http://localhost:8080/api/containerd/install
```

### Opción 2: Script Manual

```bash
# En tu Raspberry Pi
sudo ./scripts/setup_raspberry_containerd.sh
```

## Verificación

### Verificar instalación

```bash
# Script de verificación específico para Raspberry Pi
./verify_raspberry_containerd.sh
```

### Verificar desde la API

```bash
# Obtener diagnóstico completo
curl http://localhost:8080/api/containerd/diagnostic
```

## Optimización de Memoria

Para Raspberry Pi con recursos limitados:

```bash
# Aplicar optimizaciones de memoria
./optimize_raspberry_containerd.sh
```

## Comandos Útiles

```bash
# Verificar estado del servicio
sudo systemctl status containerd

# Ver logs en tiempo real
sudo journalctl -u containerd -f

# Listar contenedores
ctr -n diplo containers list

# Probar con imagen ARM
ctr -n diplo run --rm docker.io/library/hello-world:latest test

# Verificar namespaces
ctr namespaces list
```

## Configuración Automática

El sistema detectará automáticamente que estás en Raspberry Pi y:

1. **Priorizará containerd** sobre Docker
2. **Usará optimizaciones específicas** para ARM
3. **Configurará límites de memoria** apropiados
4. **Creará el namespace 'diplo'** automáticamente

## Solución de Problemas

### Error: "containerd no está corriendo"

```bash
# Verificar si el servicio está habilitado
sudo systemctl is-enabled containerd

# Habilitar e iniciar el servicio
sudo systemctl enable containerd
sudo systemctl start containerd

# Verificar logs
sudo journalctl -u containerd --no-pager -n 20
```

### Error: "ctr command not found"

```bash
# Instalar herramientas CLI
sudo apt-get install -y containerd-tools

# O descargar manualmente
curl -L -o /tmp/ctr.tar.gz "https://github.com/containerd/containerd/releases/download/v1.7.0/containerd-1.7.0-linux-arm64.tar.gz"
cd /tmp
sudo tar -xzf ctr.tar.gz bin/ctr
sudo mv bin/ctr /usr/local/bin/
sudo chmod +x /usr/local/bin/ctr
```

### Error: "Permission denied"

```bash
# Agregar usuario al grupo containerd
sudo usermod -a -G containerd $USER

# Recargar grupos (o reiniciar sesión)
newgrp containerd
```

### Problemas de Memoria

Si tu Raspberry Pi tiene poca memoria:

```bash
# Aplicar límites estrictos
sudo tee /etc/systemd/system/containerd.service.d/override.conf > /dev/null <<EOF
[Service]
MemoryLimit=256M
CPUQuota=25%
EOF

sudo systemctl daemon-reload
sudo systemctl restart containerd
```

## Configuración Avanzada

### Personalizar límites de recursos

Edita `/etc/systemd/system/containerd.service.d/override.conf`:

```ini
[Service]
MemoryLimit=512M
CPUQuota=50%
```

### Configurar almacenamiento

Para usar una tarjeta SD más rápida o almacenamiento externo:

```bash
# Crear directorio en almacenamiento externo
sudo mkdir -p /mnt/external/containerd

# Modificar configuración
sudo sed -i 's|root = "/var/lib/containerd"|root = "/mnt/external/containerd"|' /etc/containerd/config.toml

# Reiniciar containerd
sudo systemctl restart containerd
```

## Monitoreo

### Verificar uso de recursos

```bash
# Uso de memoria
free -h

# Uso de CPU
top

# Uso de disco
df -h

# Procesos de containerd
ps aux | grep containerd
```

### Logs detallados

```bash
# Logs del servicio
sudo journalctl -u containerd -f

# Logs del sistema
sudo dmesg | grep containerd

# Logs de aplicaciones
sudo journalctl -f | grep diplo
```

## Migración desde Docker

Si ya tienes Docker instalado y quieres migrar a containerd:

```bash
# 1. Hacer backup de contenedores importantes
docker ps -a

# 2. Detener Docker
sudo systemctl stop docker
sudo systemctl disable docker

# 3. Instalar containerd
sudo ./scripts/setup_raspberry_containerd.sh

# 4. Migrar imágenes (opcional)
# Las imágenes se descargarán automáticamente cuando se necesiten
```

## Rendimiento Esperado

Con containerd en Raspberry Pi 4 (4GB):

- **Uso de memoria**: ~50-100MB (vs 200-300MB de Docker)
- **Tiempo de inicio**: ~2-3 segundos (vs 5-8 segundos de Docker)
- **Uso de CPU**: ~5-10% menos que Docker
- **Espacio en disco**: ~100MB menos que Docker

## Soporte

Si encuentras problemas:

1. Ejecuta el diagnóstico: `./scripts/diagnose_containerd.sh`
2. Revisa los logs: `sudo journalctl -u containerd -f`
3. Verifica la configuración: `cat /etc/containerd/config.toml`
4. Reinicia el servicio: `sudo systemctl restart containerd`

## Próximos Pasos

1. **Reinicia tu Raspberry Pi**: `sudo reboot`
2. **Verifica la instalación**: `./verify_raspberry_containerd.sh`
3. **Inicia tu aplicación Diplo**: `make run`
4. **Prueba un deployment**: Usa la API para desplegar una aplicación

¡Tu Raspberry Pi ahora está optimizado para usar containerd con Diplo! 