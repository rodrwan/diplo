#ifndef DIPLO_H
#define DIPLO_H

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <errno.h>
#include <pthread.h>
#include <microhttpd.h>
#include <curl/curl.h>
#include <jansson.h>
#include <sqlite3.h>

// Configuración del servidor
#define DIPLO_PORT 8080
#define DIPLO_MAX_CONNECTIONS 10
#define DIPLO_BUFFER_SIZE 4096
#define DIPLO_MAX_PATH 256
#define DIPLO_MAX_URL 512
#define DIPLO_DB_PATH "diplo.db"

// Estados de la aplicación
typedef enum {
    DIPLO_STATUS_IDLE,
    DIPLO_STATUS_DEPLOYING,
    DIPLO_STATUS_RUNNING,
    DIPLO_STATUS_ERROR
} diplo_status_t;

// Estructura para una aplicación desplegada
typedef struct {
    char id[64];
    char name[128];
    char repo_url[512];
    char language[32];
    int port;
    char container_id[64];
    diplo_status_t status;
    char error_msg[256];
    time_t created_at;
    time_t updated_at;
} diplo_app_t;

// Estructura principal del servidor
typedef struct {
    struct MHD_Daemon *daemon;
    int port;
    int running;
    pthread_mutex_t apps_mutex;
    diplo_app_t *apps;
    int apps_count;
    int apps_capacity;
} diplo_server_t;

// Variable global del servidor (declarada en main.c)
extern diplo_server_t g_server;

// Funciones principales
int diplo_init(diplo_server_t *server);
void diplo_cleanup(diplo_server_t *server);
int diplo_start(diplo_server_t *server);
void diplo_stop(diplo_server_t *server);

// Funciones de manejo de aplicaciones
int diplo_add_app(diplo_server_t *server, const char *repo_url);
int diplo_remove_app(diplo_server_t *server, const char *app_id);
diplo_app_t* diplo_find_app(diplo_server_t *server, const char *app_id);

// Funciones de deployment
int diplo_deploy_app(diplo_server_t *server, diplo_app_t *app);

// Funciones de Docker
int diplo_generate_dockerfile(const char *repo_url, const char *language, const char *output_path);
int diplo_build_image(const char *dockerfile_path, const char *image_name, char *image_id);
int diplo_run_container(const char *image_name, int *port, char *container_id);
int diplo_stop_container(const char *container_id);
int diplo_remove_image(const char *image_name);
int diplo_detect_language(const char *repo_url, char *language);

// Funciones de utilidad
int diplo_find_free_port(void);
int diplo_is_port_in_use(int port);
char* diplo_generate_app_id(void);
int diplo_exec_command(const char *command, char *output, size_t output_size);
diplo_app_t* diplo_find_app(diplo_server_t *server, const char *app_id);

// Funciones de base de datos SQLite
int diplo_db_init(const char *db_path);
void diplo_db_close(void);
int diplo_db_create_tables(void);
int diplo_db_save_app(diplo_app_t *app);
int diplo_db_load_apps(diplo_server_t *server);
int diplo_db_update_app(diplo_app_t *app);
int diplo_db_delete_app(const char *app_id);
int diplo_db_log_deployment(const char *app_id, const char *action, const char *message);

// Funciones HTTP
enum MHD_Result diplo_http_handler(void *cls, struct MHD_Connection *connection,
                       const char *url, const char *method,
                       const char *version, const char *upload_data,
                       size_t *upload_data_size, void **con_cls);

// Handlers de endpoints
enum MHD_Result handler_post_deploy(struct MHD_Connection *connection,
                                   const char *upload_data, size_t upload_data_size);
enum MHD_Result handler_get_apps(struct MHD_Connection *connection);
enum MHD_Result handler_get_app_by_id(struct MHD_Connection *connection, const char *app_id);
enum MHD_Result handler_delete_app_by_id(struct MHD_Connection *connection, const char *app_id);

// Funciones auxiliares de respuesta
struct MHD_Response* create_json_response(const char *json_data, int status_code);
struct MHD_Response* create_error_response(const char *error_msg, int status_code);

#endif // DIPLO_H