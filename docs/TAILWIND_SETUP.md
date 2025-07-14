# Configuración de Tailwind CSS en Diplo

## Resumen

Este proyecto utiliza **Tailwind CSS desde CDN** para los estilos, eliminando la necesidad de Node.js y herramientas de build adicionales.

## Configuración Actual

### 1. Layout Principal (`internal/templates/Layout.templ`)

El layout principal incluye:
- **Tailwind CSS CDN**: `<script src="https://cdn.tailwindcss.com"></script>`
- **Configuración personalizada**: Colores, fuentes y animaciones personalizadas
- **Archivo CSS complementario**: `/static/components.css` con clases utilitarias

### 2. Archivo de Componentes (`internal/templates/components.css`)

Contiene clases utilitarias que complementan Tailwind:
- Botones (`.btn`, `.btn-primary`, `.btn-success`, etc.)
- Cards (`.card`)
- Formularios (`.form-group`, `.form-label`, `.form-input`)
- Indicadores de estado (`.status-indicator`, `.status-connecting`, etc.)
- Contenedores de logs (`.logs-container`, `.log-entry`, etc.)

### 3. Servidor de Archivos Estáticos

El servidor sirve archivos estáticos desde `/static/`:
```go
s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("internal/templates"))))
```

## Uso de Tailwind CSS

### Clases Básicas

```html
<!-- Grid responsivo -->
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">

<!-- Cards -->
<div class="bg-gray-700 border border-gray-600 rounded-lg p-5 shadow-lg">

<!-- Botones -->
<button class="btn btn-primary">Mi Botón</button>

<!-- Formularios -->
<input class="form-input" type="text" placeholder="Mi input">
```

### Clases Personalizadas

```html
<!-- Botones con estilos personalizados -->
<button class="btn btn-primary">Botón Primario</button>
<button class="btn btn-success">Botón Éxito</button>
<button class="btn btn-warning">Botón Advertencia</button>
<button class="btn btn-danger">Botón Peligro</button>

<!-- Cards con hover effects -->
<div class="card">
    <h3>Mi Card</h3>
    <p>Contenido de la card</p>
</div>

<!-- Indicadores de estado -->
<div class="status-indicator status-connected"></div>
<div class="status-indicator status-connecting"></div>
<div class="status-indicator status-disconnected"></div>
```

### Modales y Overlays

```html
<!-- Modal básico -->
<div class="fixed inset-0 bg-black bg-opacity-50 hidden z-50" id="myModal">
    <div class="flex items-center justify-center min-h-screen p-4">
        <div class="bg-gray-800 rounded-lg shadow-2xl w-full max-w-md">
            <div class="flex justify-between items-center p-6 border-b border-gray-600">
                <h3 class="text-xl font-semibold text-white">Título del Modal</h3>
                <button class="text-gray-400 hover:text-white text-2xl font-bold">&times;</button>
            </div>
            <div class="p-6">
                <!-- Contenido del modal -->
            </div>
        </div>
    </div>
</div>
```

### Logs y Contenedores

```html
<!-- Contenedor de logs -->
<div class="logs-container">
    <div class="log-entry log-info">Mensaje informativo</div>
    <div class="log-entry log-success">Mensaje de éxito</div>
    <div class="log-entry log-error">Mensaje de error</div>
    <div class="log-entry log-warning">Mensaje de advertencia</div>
    <div class="log-entry docker-event">Evento de Docker</div>
</div>
```

## Ventajas de esta Configuración

### ✅ Pros
- **Sin dependencias de Node.js**: No requiere npm, yarn, o herramientas de build
- **Despliegue simple**: Solo archivos estáticos servidos por Go
- **Desarrollo rápido**: Cambios instantáneos sin rebuild
- **CDN confiable**: Tailwind CSS desde CDN oficial
- **Configuración personalizada**: Colores y temas adaptados al proyecto

### ⚠️ Consideraciones
- **Dependencia de CDN**: Requiere conexión a internet para cargar Tailwind
- **Tamaño de descarga**: Tailwind completo se descarga en cada visita
- **Sin purging**: Todas las clases de Tailwind están disponibles

## Personalización

### Colores Personalizados

En `Layout.templ`, dentro del script de configuración:

```javascript
tailwind.config = {
    theme: {
        extend: {
            colors: {
                primary: {
                    50: '#eff6ff',
                    100: '#dbeafe',
                    // ... más tonos
                },
                dark: {
                    50: '#f8fafc',
                    100: '#f1f5f9',
                    // ... más tonos
                }
            }
        }
    }
}
```

### Nuevas Clases Utilitarias

Para agregar nuevas clases, edita `internal/templates/components.css`:

```css
/* Nueva clase personalizada */
.my-custom-class {
    @apply bg-blue-500 text-white px-4 py-2 rounded-lg hover:bg-blue-600 transition-colors;
}
```

## Comandos Útiles

```bash
# Generar templates
make build-templates

# Compilar proyecto completo
make build

# Ejecutar en desarrollo
make dev

# Limpiar archivos generados
make clean
```

## Migración de CSS Personalizado

Para migrar CSS personalizado a Tailwind:

1. **Identificar clases**: Buscar clases CSS personalizadas
2. **Mapear a Tailwind**: Convertir propiedades CSS a clases de Tailwind
3. **Crear utilitarias**: Para patrones complejos, agregar al archivo `components.css`
4. **Probar**: Verificar que los estilos se mantienen

### Ejemplo de Migración

**Antes (CSS personalizado):**
```css
.my-button {
    padding: 12px 24px;
    background: linear-gradient(135deg, #3498db 0%, #2980b9 100%);
    color: #ecf0f1;
    border-radius: 8px;
    font-weight: 600;
}
```

**Después (Tailwind + utilitaria):**
```html
<button class="btn btn-primary">Mi Botón</button>
```

Con la clase utilitaria definida en `components.css`:
```css
.btn-primary {
    @apply bg-gradient-to-r from-blue-500 to-blue-600 text-white hover:-translate-y-0.5 hover:shadow-lg hover:brightness-110;
}
``` 