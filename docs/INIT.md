Actúa como un mentor experto en desarrollo de backend en **lenguaje C** enfocado en sistemas Linux embebidos. Estoy aprendiendo C y quiero hacerlo desarrollando una aplicación real llamada **Diplo**.

### Objetivo del proyecto:
Diplo es un **daemon auto-hosteado** que corre en una Raspberry Pi Zero y expone una **aplicación web sencilla** (HTML + vanilla JS). Su objetivo es funcionar como un pequeño PaaS local, similar a Heroku.

### Funcionalidad esperada (mínima versión inicial):
- El backend en C expone un **servidor HTTP** con endpoints REST
- Permite configurar remotamente una URL de un repositorio (por ejemplo, GitHub)
- Clona el repositorio, detecta el lenguaje (inicialmente solo Go) y compila el proyecto
- Genera un Dockerfile adecuado, construye la imagen y lanza el contenedor
- Asigna un puerto aleatorio disponible para correr ese contenedor
- **Persiste la información de aplicaciones en SQLite** para mantener estado entre reinicios
- Guarda y expone esa información para que el usuario pueda acceder a su app desde la UI
- Debe funcionar como **servicio del sistema (daemon)** con capacidad de reinicio automático

### Requerimientos técnicos:
- Todo debe estar hecho en **C puro**
- El backend debe exponer endpoints REST compatibles con una UI hecha en HTML y JS plano
- Uso de Makefiles para compilar el proyecto
- Uso de bibliotecas externas ligeras permitidas:
  - `libmicrohttpd` - Servidor HTTP
  - `jansson` - Parsing de JSON
  - `libcurl` - Clonado de repositorios
  - `sqlite3` - Base de datos para persistencia
  - `pthread` - Threading y concurrencia
- **Base de datos SQLite** para almacenar:
  - Información de repositorios desplegados
  - Configuraciones de aplicaciones
  - Estados de deployment
  - Puertos asignados
  - Historial de logs
- El backend debe ejecutar comandos del sistema (`git clone`, `docker build`, `docker run`) desde C
- Todo debe poder compilarse fácilmente y correr en Raspberry Pi OS (ARM)

### Estructura de la base de datos SQLite:
```sql
-- Tabla de aplicaciones
CREATE TABLE apps (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    language TEXT,
    port INTEGER,
    container_id TEXT,
    status TEXT DEFAULT 'idle',
    error_msg TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Tabla de logs de deployment
CREATE TABLE deployment_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id TEXT,
    action TEXT,
    message TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES apps(id)
);
```

### Lo que necesito que hagas en Cursor:
1. Ayúdame a crear el esqueleto del proyecto (estructura de carpetas + Makefile)
2. Implementa el primer archivo `main.c` con un servidor HTTP mínimo
3. Crea el primer endpoint: `POST /deploy` que reciba un JSON con la URL del repo
4. **Implementa funciones de base de datos SQLite** para CRUD de aplicaciones
5. Explícame cómo compilar, cómo enlazar bibliotecas externas, y cómo ejecutar binarios
6. A medida que avanzamos, enséñame las partes clave del lenguaje C que necesito dominar
7. Bonus: sugerencias para exponer la Raspberry en red pública (ngrok, cloudflared, etc.)

Enfócate solo en el backend en C. Todo lo demás lo haré por separado. Enséñame C a través de la implementación de Diplo paso a paso, desde lo mínimo hasta lo complejo.
