#include "diplo.h"

// Variables globales
diplo_server_t g_server = {0};
volatile sig_atomic_t g_shutdown = 0;

// Manejador de señales para shutdown graceful
void signal_handler(int sig) {
    (void)sig; // Evitar warning de parámetro no usado
    g_shutdown = 1;
    printf("\n[INFO] Señal de shutdown recibida. Cerrando servidor...\n");
}

// Manejador HTTP principal con router
enum MHD_Result diplo_http_handler(void *cls, struct MHD_Connection *connection,
                       const char *url, const char *method,
                       const char *version, const char *upload_data,
                       size_t *upload_data_size, void **con_cls) {
    (void)cls;
    (void)version;
    
    static int dummy;
    struct MHD_Response *response;
    enum MHD_Result ret;
    
    // Manejo de OPTIONS para CORS
    if (strcmp(method, "OPTIONS") == 0) {
        response = MHD_create_response_from_buffer(0, NULL, MHD_RESPMEM_PERSISTENT);
        MHD_add_response_header(response, "Access-Control-Allow-Origin", "*");
        MHD_add_response_header(response, "Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS");
        MHD_add_response_header(response, "Access-Control-Allow-Headers", "Content-Type");
        ret = MHD_queue_response(connection, MHD_HTTP_OK, response);
        MHD_destroy_response(response);
        return ret;
    }
    
    if (&dummy != *con_cls) {
        *con_cls = &dummy;
        return MHD_YES;
    }
    
    // Para POST, necesitamos leer el body completo
    if (strcmp(method, "POST") == 0 && *upload_data_size > 0) {
        return MHD_YES; // Continuar leyendo el body
    }
    
    // Router principal
    if (strcmp(method, "POST") == 0 && strcmp(url, "/deploy") == 0) {
        return handler_post_deploy(connection, upload_data, *upload_data_size);
    } 
    else if (strcmp(method, "GET") == 0 && strcmp(url, "/apps") == 0) {
        return handler_get_apps(connection);
    }
    else if (strcmp(method, "GET") == 0 && strncmp(url, "/apps/", 6) == 0) {
        const char *app_id = url + 6; // Extraer ID después de "/apps/"
        if (strlen(app_id) > 0) {
            return handler_get_app_by_id(connection, app_id);
        }
    }
    else if (strcmp(method, "DELETE") == 0 && strncmp(url, "/apps/", 6) == 0) {
        const char *app_id = url + 6; // Extraer ID después de "/apps/"
        if (strlen(app_id) > 0) {
            return handler_delete_app_by_id(connection, app_id);
        }
    }
    else if (strcmp(method, "GET") == 0 && strcmp(url, "/") == 0) {
        // Endpoint de salud
        const char *json_response = "{\"status\":\"ok\",\"message\":\"Diplo server running\",\"version\":\"1.0.0\"}";
        response = MHD_create_response_from_buffer(strlen(json_response),
                                                 (void*)json_response,
                                                 MHD_RESPMEM_PERSISTENT);
        MHD_add_response_header(response, "Content-Type", "application/json");
        MHD_add_response_header(response, "Access-Control-Allow-Origin", "*");
        ret = MHD_queue_response(connection, MHD_HTTP_OK, response);
        MHD_destroy_response(response);
        return ret;
    }
    
    // 404 Not Found
    response = create_error_response("Endpoint no encontrado", MHD_HTTP_NOT_FOUND);
    ret = MHD_queue_response(connection, MHD_HTTP_NOT_FOUND, response);
    MHD_destroy_response(response);
    return ret;
}

// Inicialización del servidor
int diplo_init(diplo_server_t *server) {
    memset(server, 0, sizeof(diplo_server_t));
    
    server->port = DIPLO_PORT;
    server->running = 0;
    server->apps_count = 0;
    server->apps_capacity = 10;
    
    // Inicializar mutex
    if (pthread_mutex_init(&server->apps_mutex, NULL) != 0) {
        fprintf(stderr, "[ERROR] No se pudo inicializar el mutex\n");
        return -1;
    }
    
    // Inicializar array de aplicaciones
    server->apps = malloc(server->apps_capacity * sizeof(diplo_app_t));
    if (server->apps == NULL) {
        fprintf(stderr, "[ERROR] No se pudo asignar memoria para aplicaciones\n");
        pthread_mutex_destroy(&server->apps_mutex);
        return -1;
    }
    
    // Inicializar base de datos
    if (diplo_db_init(DIPLO_DB_PATH) != 0) {
        fprintf(stderr, "[ERROR] No se pudo inicializar la base de datos\n");
        free(server->apps);
        pthread_mutex_destroy(&server->apps_mutex);
        return -1;
    }
    
    // Cargar aplicaciones desde la base de datos
    if (diplo_db_load_apps(server) != 0) {
        fprintf(stderr, "[WARNING] No se pudieron cargar aplicaciones desde la BD\n");
    }
    
    return 0;
}

// Limpieza del servidor
void diplo_cleanup(diplo_server_t *server) {
    if (server->apps) {
        free(server->apps);
        server->apps = NULL;
    }
    
    pthread_mutex_destroy(&server->apps_mutex);
    
    // Cerrar base de datos
    diplo_db_close();
}

// Iniciar el servidor
int diplo_start(diplo_server_t *server) {
    server->daemon = MHD_start_daemon(MHD_USE_SELECT_INTERNALLY,
                                      server->port,
                                      NULL, NULL,
                                      &diplo_http_handler, NULL,
                                      MHD_OPTION_END);
    
    if (server->daemon == NULL) {
        fprintf(stderr, "[ERROR] No se pudo iniciar el servidor HTTP en puerto %d\n", server->port);
        return -1;
    }
    
    server->running = 1;
    printf("[INFO] Servidor Diplo iniciado en puerto %d\n", server->port);
    printf("[INFO] Presiona Ctrl+C para detener el servidor\n");
    
    return 0;
}

// Detener el servidor
void diplo_stop(diplo_server_t *server) {
    if (server->daemon) {
        MHD_stop_daemon(server->daemon);
        server->daemon = NULL;
    }
    
    server->running = 0;
    printf("[INFO] Servidor detenido\n");
}

// Función principal
int main(int argc, char *argv[]) {
    (void)argc;
    (void)argv;
    
    printf("=== Diplo - PaaS Local en C ===\n");
    printf("Iniciando servidor...\n");
    
    // Inicializar generador de números aleatorios para puertos
    srand(time(NULL));
    
    // Configurar manejador de señales
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);
    
    // Inicializar servidor
    if (diplo_init(&g_server) != 0) {
        fprintf(stderr, "[ERROR] Fallo en la inicialización del servidor\n");
        return 1;
    }
    
    // Iniciar servidor HTTP
    if (diplo_start(&g_server) != 0) {
        fprintf(stderr, "[ERROR] Fallo al iniciar el servidor HTTP\n");
        diplo_cleanup(&g_server);
        return 1;
    }
    
    // Loop principal
    while (!g_shutdown && g_server.running) {
        sleep(1);
    }
    
    // Limpieza
    diplo_stop(&g_server);
    diplo_cleanup(&g_server);
    
    printf("[INFO] Diplo terminado correctamente\n");
    return 0;
} 