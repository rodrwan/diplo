#include "diplo.h"

// Función auxiliar para crear respuesta JSON
struct MHD_Response* create_json_response(const char *json_data, int status_code) {
    struct MHD_Response *response = MHD_create_response_from_buffer(
        strlen(json_data), (void*)json_data, MHD_RESPMEM_PERSISTENT);

    if (response) {
        MHD_add_response_header(response, "Content-Type", "application/json");
        MHD_add_response_header(response, "Access-Control-Allow-Origin", "*");
        MHD_add_response_header(response, "Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS");
        MHD_add_response_header(response, "Access-Control-Allow-Headers", "Content-Type");
    }

    return response;
}

// Función auxiliar para crear respuesta de error
struct MHD_Response* create_error_response(const char *error_msg, int status_code) {
    char json_error[512];
    snprintf(json_error, sizeof(json_error),
             "{\"error\":\"%s\",\"status\":\"error\"}", error_msg);

    return create_json_response(json_error, status_code);
}

// Handler para POST /deploy
enum MHD_Result handler_post_deploy(struct MHD_Connection *connection,
                                   const char *upload_data, size_t upload_data_size) {
    json_t *json = NULL;
    json_error_t error;
    struct MHD_Response *response;
    enum MHD_Result ret;

    // Parsear JSON del body
    json = json_loads(upload_data, 0, &error);
    if (!json) {
        response = create_error_response("JSON inválido", MHD_HTTP_BAD_REQUEST);
        ret = MHD_queue_response(connection, MHD_HTTP_BAD_REQUEST, response);
        MHD_destroy_response(response);
        return ret;
    }

    // Extraer repo_url del JSON
    json_t *repo_url_json = json_object_get(json, "repo_url");
    if (!repo_url_json || !json_is_string(repo_url_json)) {
        json_decref(json);
        response = create_error_response("repo_url es requerido", MHD_HTTP_BAD_REQUEST);
        ret = MHD_queue_response(connection, MHD_HTTP_BAD_REQUEST, response);
        MHD_destroy_response(response);
        return ret;
    }

    const char *repo_url = json_string_value(repo_url_json);

    // Extraer name (opcional)
    json_t *name_json = json_object_get(json, "name");
    const char *app_name = name_json && json_is_string(name_json) ?
                          json_string_value(name_json) : NULL;

    // Crear nueva aplicación
    diplo_app_t new_app = {0};
    char *app_id = diplo_generate_app_id();
    strncpy(new_app.id, app_id, sizeof(new_app.id) - 1);
    strncpy(new_app.repo_url, repo_url, sizeof(new_app.repo_url) - 1);

    if (app_name) {
        strncpy(new_app.name, app_name, sizeof(new_app.name) - 1);
    } else {
        // Generar nombre basado en la URL del repo
        const char *last_slash = strrchr(repo_url, '/');
        if (last_slash && *(last_slash + 1)) {
            strncpy(new_app.name, last_slash + 1, sizeof(new_app.name) - 1);
            // Remover .git si existe
            char *dot_git = strstr(new_app.name, ".git");
            if (dot_git) *dot_git = '\0';
        } else {
            strncpy(new_app.name, "app", sizeof(new_app.name) - 1);
        }
    }

    // Asignar puerto libre al crear la aplicación
    new_app.port = diplo_find_free_port();
    if (new_app.port == -1) {
        json_decref(json);
        free(app_id);
        response = create_error_response("No se pudo asignar puerto libre", MHD_HTTP_INTERNAL_SERVER_ERROR);
        ret = MHD_queue_response(connection, MHD_HTTP_INTERNAL_SERVER_ERROR, response);
        MHD_destroy_response(response);
        return ret;
    }

    new_app.status = DIPLO_STATUS_IDLE;
    new_app.created_at = time(NULL);
    new_app.updated_at = time(NULL);

    // Guardar en base de datos
    if (diplo_db_save_app(&new_app) != 0) {
        json_decref(json);
        free(app_id);
        response = create_error_response("Error guardando aplicación", MHD_HTTP_INTERNAL_SERVER_ERROR);
        ret = MHD_queue_response(connection, MHD_HTTP_INTERNAL_SERVER_ERROR, response);
        MHD_destroy_response(response);
        return ret;
    }

        // Log del deployment
    diplo_db_log_deployment(new_app.id, "created", "Aplicación creada");

    // Agregar la aplicación al servidor
    pthread_mutex_lock(&g_server.apps_mutex);

    // Expandir array si es necesario
    if (g_server.apps_count >= g_server.apps_capacity) {
        g_server.apps_capacity *= 2;
        diplo_app_t *new_apps = realloc(g_server.apps, g_server.apps_capacity * sizeof(diplo_app_t));
        if (!new_apps) {
            pthread_mutex_unlock(&g_server.apps_mutex);
            json_decref(json);
            free(app_id);
            response = create_error_response("Error de memoria", MHD_HTTP_INTERNAL_SERVER_ERROR);
            ret = MHD_queue_response(connection, MHD_HTTP_INTERNAL_SERVER_ERROR, response);
            MHD_destroy_response(response);
            return ret;
        }
        g_server.apps = new_apps;
    }

    g_server.apps[g_server.apps_count] = new_app;
    g_server.apps_count++;

    pthread_mutex_unlock(&g_server.apps_mutex);

    // Iniciar deployment en background (en una implementación real, usaríamos threads)
    printf("[INFO] Iniciando deployment de: %s (%s)\n", new_app.name, new_app.id);

    // Por ahora, ejecutar deployment de forma síncrona
    if (diplo_deploy_app(&g_server, &new_app) != 0) {
        printf("[WARNING] Deployment falló para: %s\n", new_app.id);
    }

    // Crear respuesta JSON con URL de acceso
    char response_json[1024];
    snprintf(response_json, sizeof(response_json),
             "{\"id\":\"%s\",\"name\":\"%s\",\"repo_url\":\"%s\","
             "\"port\":%d,\"url\":\"http://localhost:%d\","
             "\"status\":\"deploying\",\"message\":\"Aplicación creada y deployment iniciado\"}",
             new_app.id, new_app.name, new_app.repo_url, new_app.port, new_app.port);

    response = create_json_response(response_json, MHD_HTTP_CREATED);
    ret = MHD_queue_response(connection, MHD_HTTP_CREATED, response);
    MHD_destroy_response(response);

    json_decref(json);
    free(app_id);

    printf("[INFO] Nueva aplicación creada: %s (%s)\n", new_app.name, new_app.id);
    return ret;
}

// Handler para GET /apps
enum MHD_Result handler_get_apps(struct MHD_Connection *connection) {
    struct MHD_Response *response;
    enum MHD_Result ret;

    // Crear array JSON de aplicaciones
    json_t *apps_array = json_array();

    pthread_mutex_lock(&g_server.apps_mutex);

    for (int i = 0; i < g_server.apps_count; i++) {
        diplo_app_t *app = &g_server.apps[i];

        json_t *app_obj = json_object();
        json_object_set_new(app_obj, "id", json_string(app->id));
        json_object_set_new(app_obj, "name", json_string(app->name));
        json_object_set_new(app_obj, "repo_url", json_string(app->repo_url));
        json_object_set_new(app_obj, "language", json_string(app->language));
        json_object_set_new(app_obj, "port", json_integer(app->port));

        // Generar URL de acceso
        char access_url[256];
        snprintf(access_url, sizeof(access_url), "http://localhost:%d", app->port);
        json_object_set_new(app_obj, "url", json_string(access_url));

        json_object_set_new(app_obj, "container_id", json_string(app->container_id));

        const char *status_str = (app->status == DIPLO_STATUS_IDLE) ? "idle" :
                                (app->status == DIPLO_STATUS_DEPLOYING) ? "deploying" :
                                (app->status == DIPLO_STATUS_RUNNING) ? "running" : "error";
        json_object_set_new(app_obj, "status", json_string(status_str));

        json_object_set_new(app_obj, "error_msg", json_string(app->error_msg));
        json_object_set_new(app_obj, "created_at", json_integer(app->created_at));
        json_object_set_new(app_obj, "updated_at", json_integer(app->updated_at));

        json_array_append_new(apps_array, app_obj);
    }

    pthread_mutex_unlock(&g_server.apps_mutex);

    char *json_string = json_dumps(apps_array, JSON_INDENT(2));
    response = create_json_response(json_string, MHD_HTTP_OK);
    ret = MHD_queue_response(connection, MHD_HTTP_OK, response);
    MHD_destroy_response(response);

    free(json_string);
    json_decref(apps_array);

    return ret;
}

// Handler para GET /apps/{id}
enum MHD_Result handler_get_app_by_id(struct MHD_Connection *connection, const char *app_id) {
    struct MHD_Response *response;
    enum MHD_Result ret;

    pthread_mutex_lock(&g_server.apps_mutex);

    diplo_app_t *app = diplo_find_app(&g_server, app_id);
    if (!app) {
        pthread_mutex_unlock(&g_server.apps_mutex);
        response = create_error_response("Aplicación no encontrada", MHD_HTTP_NOT_FOUND);
        ret = MHD_queue_response(connection, MHD_HTTP_NOT_FOUND, response);
        MHD_destroy_response(response);
        return ret;
    }

    // Crear objeto JSON de la aplicación
    json_t *app_obj = json_object();
    json_object_set_new(app_obj, "id", json_string(app->id));
    json_object_set_new(app_obj, "name", json_string(app->name));
    json_object_set_new(app_obj, "repo_url", json_string(app->repo_url));
    json_object_set_new(app_obj, "language", json_string(app->language));
    json_object_set_new(app_obj, "port", json_integer(app->port));

    // Generar URL de acceso
    char access_url[256];
    snprintf(access_url, sizeof(access_url), "http://localhost:%d", app->port);
    json_object_set_new(app_obj, "url", json_string(access_url));

    json_object_set_new(app_obj, "container_id", json_string(app->container_id));

    const char *status_str = (app->status == DIPLO_STATUS_IDLE) ? "idle" :
                            (app->status == DIPLO_STATUS_DEPLOYING) ? "deploying" :
                            (app->status == DIPLO_STATUS_RUNNING) ? "running" : "error";
    json_object_set_new(app_obj, "status", json_string(status_str));

    json_object_set_new(app_obj, "error_msg", json_string(app->error_msg));
    json_object_set_new(app_obj, "created_at", json_integer(app->created_at));
    json_object_set_new(app_obj, "updated_at", json_integer(app->updated_at));

    pthread_mutex_unlock(&g_server.apps_mutex);

    char *json_string = json_dumps(app_obj, JSON_INDENT(2));
    response = create_json_response(json_string, MHD_HTTP_OK);
    ret = MHD_queue_response(connection, MHD_HTTP_OK, response);
    MHD_destroy_response(response);

    free(json_string);
    json_decref(app_obj);

    return ret;
}

// Handler para DELETE /apps/{id}
enum MHD_Result handler_delete_app_by_id(struct MHD_Connection *connection, const char *app_id) {
    struct MHD_Response *response;
    enum MHD_Result ret;

    pthread_mutex_lock(&g_server.apps_mutex);

    diplo_app_t *app = diplo_find_app(&g_server, app_id);
    if (!app) {
        pthread_mutex_unlock(&g_server.apps_mutex);
        response = create_error_response("Aplicación no encontrada", MHD_HTTP_NOT_FOUND);
        ret = MHD_queue_response(connection, MHD_HTTP_NOT_FOUND, response);
        MHD_destroy_response(response);
        return ret;
    }

    // Si hay un contenedor corriendo, detenerlo
    if (strlen(app->container_id) > 0) {
        char stop_cmd[256];
        snprintf(stop_cmd, sizeof(stop_cmd), "docker stop %s", app->container_id);
        system(stop_cmd);

        char rm_cmd[256];
        snprintf(rm_cmd, sizeof(rm_cmd), "docker rm %s", app->container_id);
        system(rm_cmd);
    }

    // Eliminar de la base de datos
    if (diplo_db_delete_app(app_id) != 0) {
        pthread_mutex_unlock(&g_server.apps_mutex);
        response = create_error_response("Error eliminando aplicación", MHD_HTTP_INTERNAL_SERVER_ERROR);
        ret = MHD_queue_response(connection, MHD_HTTP_INTERNAL_SERVER_ERROR, response);
        MHD_destroy_response(response);
        return ret;
    }

    // Log del deletion
    diplo_db_log_deployment(app_id, "deleted", "Aplicación eliminada");

    // Eliminar del array en memoria
    for (int i = 0; i < g_server.apps_count; i++) {
        if (strcmp(g_server.apps[i].id, app_id) == 0) {
            // Mover elementos restantes
            for (int j = i; j < g_server.apps_count - 1; j++) {
                g_server.apps[j] = g_server.apps[j + 1];
            }
            g_server.apps_count--;
            break;
        }
    }

    pthread_mutex_unlock(&g_server.apps_mutex);

    char response_json[256];
    snprintf(response_json, sizeof(response_json),
             "{\"message\":\"Aplicación eliminada exitosamente\",\"id\":\"%s\"}", app_id);

    response = create_json_response(response_json, MHD_HTTP_OK);
    ret = MHD_queue_response(connection, MHD_HTTP_OK, response);
    MHD_destroy_response(response);

    printf("[INFO] Aplicación eliminada: %s\n", app_id);
    return ret;
}