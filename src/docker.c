#include "diplo.h"
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

// Generar Dockerfile para Go
int diplo_generate_go_dockerfile(const char *repo_url, const char *output_path) {
    FILE *file = fopen(output_path, "w");
    if (!file) {
        fprintf(stderr, "[ERROR] No se pudo crear Dockerfile en %s\n", output_path);
        return -1;
    }

    fprintf(file, "# Diplo - Dockerfile generado automáticamente\n");
    fprintf(file, "FROM golang:1.24-alpine AS builder\n");
    fprintf(file, "WORKDIR /app\n");
    fprintf(file, "RUN apk add --no-cache git\n");
    fprintf(file, "RUN git clone %s .\n", repo_url);
    fprintf(file, "RUN go mod download\n");
    fprintf(file, "RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .\n");
    fprintf(file, "\n");
    fprintf(file, "FROM alpine:latest\n");
    fprintf(file, "RUN apk --no-cache add ca-certificates\n");
    fprintf(file, "WORKDIR /root/\n");
    fprintf(file, "COPY --from=builder /app/main .\n");
    fprintf(file, "EXPOSE 8080\n");
    fprintf(file, "CMD [\"./main\"]\n");

    fclose(file);
    printf("[INFO] Dockerfile generado para Go: %s\n", output_path);
    return 0;
}

// Generar Dockerfile para Node.js
int diplo_generate_node_dockerfile(const char *repo_url, const char *output_path) {
    FILE *file = fopen(output_path, "w");
    if (!file) {
        fprintf(stderr, "[ERROR] No se pudo crear Dockerfile en %s\n", output_path);
        return -1;
    }

    fprintf(file, "# Diplo - Dockerfile generado automáticamente\n");
    fprintf(file, "FROM node:18-alpine AS builder\n");
    fprintf(file, "WORKDIR /app\n");
    fprintf(file, "RUN apk add --no-cache git\n");
    fprintf(file, "RUN git clone %s .\n", repo_url);
    fprintf(file, "RUN npm ci --only=production\n");
    fprintf(file, "\n");
    fprintf(file, "FROM node:18-alpine\n");
    fprintf(file, "WORKDIR /app\n");
    fprintf(file, "COPY --from=builder /app .\n");
    fprintf(file, "EXPOSE 3000\n");
    fprintf(file, "CMD [\"npm\", \"start\"]\n");

    fclose(file);
    printf("[INFO] Dockerfile generado para Node.js: %s\n", output_path);
    return 0;
}

// Generar Dockerfile para Python
int diplo_generate_python_dockerfile(const char *repo_url, const char *output_path) {
    FILE *file = fopen(output_path, "w");
    if (!file) {
        fprintf(stderr, "[ERROR] No se pudo crear Dockerfile en %s\n", output_path);
        return -1;
    }

    fprintf(file, "# Diplo - Dockerfile generado automáticamente\n");
    fprintf(file, "FROM python:3.11-alpine AS builder\n");
    fprintf(file, "WORKDIR /app\n");
    fprintf(file, "RUN apk add --no-cache git\n");
    fprintf(file, "RUN git clone %s .\n", repo_url);
    fprintf(file, "RUN pip install -r requirements.txt\n");
    fprintf(file, "\n");
    fprintf(file, "FROM python:3.11-alpine\n");
    fprintf(file, "WORKDIR /app\n");
    fprintf(file, "COPY --from=builder /app .\n");
    fprintf(file, "EXPOSE 8000\n");
    fprintf(file, "CMD [\"python\", \"app.py\"]\n");

    fclose(file);
    printf("[INFO] Dockerfile generado para Python: %s\n", output_path);
    return 0;
}

// Función principal para generar Dockerfile según el lenguaje
int diplo_generate_dockerfile(const char *repo_url, const char *language, const char *output_path) {
    if (strcmp(language, "go") == 0) {
        return diplo_generate_go_dockerfile(repo_url, output_path);
    } else if (strcmp(language, "node") == 0 || strcmp(language, "javascript") == 0) {
        return diplo_generate_node_dockerfile(repo_url, output_path);
    } else if (strcmp(language, "python") == 0) {
        return diplo_generate_python_dockerfile(repo_url, output_path);
    } else {
        fprintf(stderr, "[ERROR] Lenguaje no soportado: %s\n", language);
        return -1;
    }
}

// Construir imagen Docker usando la API
int diplo_build_image(const char *dockerfile_path, const char *image_name, char *image_id) {
    char build_cmd[1024];
    char output[4096];

    // Comando para construir la imagen
    snprintf(build_cmd, sizeof(build_cmd),
             "docker build -t %s %s", image_name, dockerfile_path);

    printf("[INFO] Construyendo imagen: %s\n", build_cmd);

    // Ejecutar el comando
    int result = diplo_exec_command(build_cmd, output, sizeof(output));
    if (result != 0) {
        fprintf(stderr, "[ERROR] Error construyendo imagen: %s\n", output);
        return -1;
    }

    // Obtener el ID de la imagen
    char inspect_cmd[512];
    snprintf(inspect_cmd, sizeof(inspect_cmd),
             "docker images --format '{{.ID}}' %s", image_name);

    char image_id_output[128];
    if (diplo_exec_command(inspect_cmd, image_id_output, sizeof(image_id_output)) == 0) {
        // Remover newline del final
        char *newline = strchr(image_id_output, '\n');
        if (newline) *newline = '\0';
        strncpy(image_id, image_id_output, 64);
    } else {
        strncpy(image_id, "unknown", 64);
    }

    printf("[INFO] Imagen construida exitosamente: %s (ID: %s)\n", image_name, image_id);
    return 0;
}

// Ejecutar contenedor Docker
int diplo_run_container(const char *image_name, int *port, char *container_id) {
    // Usar el puerto pre-asignado
    if (*port == 0) {
        fprintf(stderr, "[ERROR] Puerto no asignado\n");
        return -1;
    }

    char run_cmd[1024];
    char output[4096];

    // Comando para ejecutar el contenedor
    snprintf(run_cmd, sizeof(run_cmd),
             "docker run -d -p %d:8080 --name diplo_%s_%d %s",
             *port, image_name, *port, image_name);

    printf("[INFO] Ejecutando contenedor: %s\n", run_cmd);

    // Ejecutar el comando
    int result = diplo_exec_command(run_cmd, output, sizeof(output));
    if (result != 0) {
        fprintf(stderr, "[ERROR] Error ejecutando contenedor: %s\n", output);
        return -1;
    }

    // Obtener el ID del contenedor
    char inspect_cmd[512];
    snprintf(inspect_cmd, sizeof(inspect_cmd),
             "docker ps --format '{{.ID}}' --filter 'name=diplo_%s_%d'", image_name, *port);

    char container_id_output[128];
    if (diplo_exec_command(inspect_cmd, container_id_output, sizeof(container_id_output)) == 0) {
        // Remover newline del final
        char *newline = strchr(container_id_output, '\n');
        if (newline) *newline = '\0';
        strncpy(container_id, container_id_output, 64);
    } else {
        strncpy(container_id, "unknown", 64);
    }

    printf("[INFO] Contenedor ejecutado exitosamente: %s (ID: %s, Puerto: %d)\n",
           image_name, container_id, *port);
    return 0;
}

// Detener y eliminar contenedor
int diplo_stop_container(const char *container_id) {
    char stop_cmd[256];
    char rm_cmd[256];

    // Detener contenedor
    snprintf(stop_cmd, sizeof(stop_cmd), "docker stop %s", container_id);
    diplo_exec_command(stop_cmd, NULL, 0);

    // Eliminar contenedor
    snprintf(rm_cmd, sizeof(rm_cmd), "docker rm %s", container_id);
    diplo_exec_command(rm_cmd, NULL, 0);

    printf("[INFO] Contenedor detenido y eliminado: %s\n", container_id);
    return 0;
}

// Eliminar imagen Docker
int diplo_remove_image(const char *image_name) {
    char rmi_cmd[256];
    snprintf(rmi_cmd, sizeof(rmi_cmd), "docker rmi %s", image_name);

    int result = diplo_exec_command(rmi_cmd, NULL, 0);
    if (result == 0) {
        printf("[INFO] Imagen eliminada: %s\n", image_name);
    }
    return result;
}

// Detectar lenguaje del repositorio (simplificado)
int diplo_detect_language(const char *repo_url, char *language) {
    // Por ahora, vamos a detectar basándonos en la URL o usar Go por defecto
    // En una implementación real, clonaríamos temporalmente y buscaríamos archivos específicos

    if (strstr(repo_url, "go") != NULL || strstr(repo_url, "golang") != NULL) {
        strncpy(language, "go", 32);
    } else if (strstr(repo_url, "node") != NULL || strstr(repo_url, "js") != NULL ||
               strstr(repo_url, "javascript") != NULL) {
        strncpy(language, "node", 32);
    } else if (strstr(repo_url, "python") != NULL || strstr(repo_url, "py") != NULL) {
        strncpy(language, "python", 32);
    } else {
        // Por defecto, usar Go
        strncpy(language, "go", 32);
    }

    printf("[INFO] Lenguaje detectado: %s para repo: %s\n", language, repo_url);
    return 0;
}

// Función principal de deployment
int diplo_deploy_app(diplo_server_t *server, diplo_app_t *app) {
    char language[32];
    char dockerfile_path[256];
    char image_name[128];
    char image_id[64];
    char container_id[64];
    int port;

    printf("[INFO] Iniciando deployment de aplicación: %s\n", app->id);

    // Log del inicio del deployment
    diplo_db_log_deployment(app->id, "deploy_start", "Iniciando deployment");

    // Actualizar estado a deploying
    app->status = DIPLO_STATUS_DEPLOYING;
    diplo_db_update_app(app);

    // 1. Detectar lenguaje
    if (diplo_detect_language(app->repo_url, language) != 0) {
        strcpy(app->error_msg, "Error detectando lenguaje");
        app->status = DIPLO_STATUS_ERROR;
        diplo_db_update_app(app);
        diplo_db_log_deployment(app->id, "deploy_error", "Error detectando lenguaje");
        return -1;
    }

    strncpy(app->language, language, sizeof(app->language) - 1);

    // 2. Generar Dockerfile
    snprintf(dockerfile_path, sizeof(dockerfile_path), "/tmp/diplo_%s.Dockerfile", app->id);
    if (diplo_generate_dockerfile(app->repo_url, language, dockerfile_path) != 0) {
        strcpy(app->error_msg, "Error generando Dockerfile");
        app->status = DIPLO_STATUS_ERROR;
        diplo_db_update_app(app);
        diplo_db_log_deployment(app->id, "deploy_error", "Error generando Dockerfile");
        return -1;
    }

    // 3. Construir imagen Docker
    snprintf(image_name, sizeof(image_name), "diplo_%s", app->id);
    if (diplo_build_image(dockerfile_path, image_name, image_id) != 0) {
        strcpy(app->error_msg, "Error construyendo imagen Docker");
        app->status = DIPLO_STATUS_ERROR;
        diplo_db_update_app(app);
        diplo_db_log_deployment(app->id, "deploy_error", "Error construyendo imagen Docker");
        return -1;
    }

        // 4. Ejecutar contenedor usando el puerto pre-asignado
    port = app->port; // Usar el puerto que ya fue asignado
    if (diplo_run_container(image_name, &port, container_id) != 0) {
        strcpy(app->error_msg, "Error ejecutando contenedor");
        app->status = DIPLO_STATUS_ERROR;
        diplo_db_update_app(app);
        diplo_db_log_deployment(app->id, "deploy_error", "Error ejecutando contenedor");
        return -1;
    }
    
    // 5. Actualizar aplicación con datos del deployment
    // El puerto ya está asignado, solo actualizar container_id
    strncpy(app->container_id, container_id, sizeof(app->container_id) - 1);
    app->status = DIPLO_STATUS_RUNNING;
    app->updated_at = time(NULL);
    strcpy(app->error_msg, ""); // Limpiar errores previos

    // 6. Guardar en base de datos
    if (diplo_db_update_app(app) != 0) {
        fprintf(stderr, "[ERROR] Error actualizando aplicación en BD\n");
    }

    // 7. Log del éxito
    char success_msg[256];
    snprintf(success_msg, sizeof(success_msg),
             "Deployment exitoso - Puerto: %d, Container: %s", port, container_id);
    diplo_db_log_deployment(app->id, "deploy_success", success_msg);

    printf("[INFO] Deployment completado exitosamente: %s en puerto %d\n", app->id, port);

    // Limpiar archivo temporal
    unlink(dockerfile_path);

    return 0;
}