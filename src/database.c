#include "diplo.h"

// Variable global para la conexión de la base de datos
sqlite3 *g_db = NULL;

// Inicializar la base de datos
int diplo_db_init(const char *db_path) {
    int rc;
    
    // Abrir conexión a la base de datos
    rc = sqlite3_open(db_path, &g_db);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] No se pudo abrir la base de datos: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    printf("[INFO] Base de datos SQLite inicializada: %s\n", db_path);
    
    // Crear tablas si no existen
    if (diplo_db_create_tables() != 0) {
        fprintf(stderr, "[ERROR] No se pudieron crear las tablas\n");
        sqlite3_close(g_db);
        g_db = NULL;
        return -1;
    }
    
    return 0;
}

// Cerrar la base de datos
void diplo_db_close(void) {
    if (g_db) {
        sqlite3_close(g_db);
        g_db = NULL;
        printf("[INFO] Base de datos cerrada\n");
    }
}

// Crear tablas de la base de datos
int diplo_db_create_tables(void) {
    const char *create_apps_table = 
        "CREATE TABLE IF NOT EXISTS apps ("
        "id TEXT PRIMARY KEY,"
        "name TEXT NOT NULL,"
        "repo_url TEXT NOT NULL,"
        "language TEXT,"
        "port INTEGER,"
        "container_id TEXT,"
        "status TEXT DEFAULT 'idle',"
        "error_msg TEXT,"
        "created_at DATETIME DEFAULT CURRENT_TIMESTAMP,"
        "updated_at DATETIME DEFAULT CURRENT_TIMESTAMP"
        ");";
    
    const char *create_logs_table = 
        "CREATE TABLE IF NOT EXISTS deployment_logs ("
        "id INTEGER PRIMARY KEY AUTOINCREMENT,"
        "app_id TEXT,"
        "action TEXT,"
        "message TEXT,"
        "timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,"
        "FOREIGN KEY (app_id) REFERENCES apps(id)"
        ");";
    
    char *err_msg = 0;
    int rc;
    
    // Crear tabla de aplicaciones
    rc = sqlite3_exec(g_db, create_apps_table, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] Error al crear tabla apps: %s\n", err_msg);
        sqlite3_free(err_msg);
        return -1;
    }
    
    // Crear tabla de logs
    rc = sqlite3_exec(g_db, create_logs_table, 0, 0, &err_msg);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] Error al crear tabla logs: %s\n", err_msg);
        sqlite3_free(err_msg);
        return -1;
    }
    
    printf("[INFO] Tablas de base de datos creadas correctamente\n");
    return 0;
}

// Guardar una aplicación en la base de datos
int diplo_db_save_app(diplo_app_t *app) {
    const char *sql = 
        "INSERT OR REPLACE INTO apps "
        "(id, name, repo_url, language, port, container_id, status, error_msg, updated_at) "
        "VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP);";
    
    sqlite3_stmt *stmt;
    int rc;
    
    rc = sqlite3_prepare_v2(g_db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] Error preparando statement: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    // Bind de parámetros
    sqlite3_bind_text(stmt, 1, app->id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 2, app->name, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 3, app->repo_url, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 4, app->language, -1, SQLITE_STATIC);
    sqlite3_bind_int(stmt, 5, app->port);
    sqlite3_bind_text(stmt, 6, app->container_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 7, (app->status == DIPLO_STATUS_IDLE) ? "idle" :
                      (app->status == DIPLO_STATUS_DEPLOYING) ? "deploying" :
                      (app->status == DIPLO_STATUS_RUNNING) ? "running" : "error", -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 8, app->error_msg, -1, SQLITE_STATIC);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    if (rc != SQLITE_DONE) {
        fprintf(stderr, "[ERROR] Error guardando aplicación: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    printf("[INFO] Aplicación guardada en BD: %s\n", app->id);
    return 0;
}

// Cargar todas las aplicaciones desde la base de datos
int diplo_db_load_apps(diplo_server_t *server) {
    const char *sql = "SELECT id, name, repo_url, language, port, container_id, status, error_msg, created_at FROM apps;";
    
    sqlite3_stmt *stmt;
    int rc;
    
    rc = sqlite3_prepare_v2(g_db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] Error preparando statement: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    server->apps_count = 0;
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        // Expandir array si es necesario
        if (server->apps_count >= server->apps_capacity) {
            server->apps_capacity *= 2;
            diplo_app_t *new_apps = realloc(server->apps, server->apps_capacity * sizeof(diplo_app_t));
            if (!new_apps) {
                fprintf(stderr, "[ERROR] No se pudo expandir el array de aplicaciones\n");
                sqlite3_finalize(stmt);
                return -1;
            }
            server->apps = new_apps;
        }
        
        diplo_app_t *app = &server->apps[server->apps_count];
        
        // Cargar datos desde la BD
        strncpy(app->id, (const char*)sqlite3_column_text(stmt, 0), sizeof(app->id) - 1);
        strncpy(app->name, (const char*)sqlite3_column_text(stmt, 1), sizeof(app->name) - 1);
        strncpy(app->repo_url, (const char*)sqlite3_column_text(stmt, 2), sizeof(app->repo_url) - 1);
        strncpy(app->language, (const char*)sqlite3_column_text(stmt, 3), sizeof(app->language) - 1);
        app->port = sqlite3_column_int(stmt, 4);
        strncpy(app->container_id, (const char*)sqlite3_column_text(stmt, 5), sizeof(app->container_id) - 1);
        
        // Convertir status string a enum
        const char *status_str = (const char*)sqlite3_column_text(stmt, 6);
        if (strcmp(status_str, "idle") == 0) app->status = DIPLO_STATUS_IDLE;
        else if (strcmp(status_str, "deploying") == 0) app->status = DIPLO_STATUS_DEPLOYING;
        else if (strcmp(status_str, "running") == 0) app->status = DIPLO_STATUS_RUNNING;
        else app->status = DIPLO_STATUS_ERROR;
        
        strncpy(app->error_msg, (const char*)sqlite3_column_text(stmt, 7), sizeof(app->error_msg) - 1);
        app->created_at = sqlite3_column_int64(stmt, 8);
        app->updated_at = time(NULL);
        
        server->apps_count++;
    }
    
    sqlite3_finalize(stmt);
    printf("[INFO] Cargadas %d aplicaciones desde la base de datos\n", server->apps_count);
    return 0;
}

// Actualizar una aplicación en la base de datos
int diplo_db_update_app(diplo_app_t *app) {
    return diplo_db_save_app(app); // SQLite usa INSERT OR REPLACE
}

// Eliminar una aplicación de la base de datos
int diplo_db_delete_app(const char *app_id) {
    const char *sql = "DELETE FROM apps WHERE id = ?;";
    
    sqlite3_stmt *stmt;
    int rc;
    
    rc = sqlite3_prepare_v2(g_db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] Error preparando statement: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    sqlite3_bind_text(stmt, 1, app_id, -1, SQLITE_STATIC);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    if (rc != SQLITE_DONE) {
        fprintf(stderr, "[ERROR] Error eliminando aplicación: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    printf("[INFO] Aplicación eliminada de BD: %s\n", app_id);
    return 0;
}

// Registrar un log de deployment
int diplo_db_log_deployment(const char *app_id, const char *action, const char *message) {
    const char *sql = "INSERT INTO deployment_logs (app_id, action, message) VALUES (?, ?, ?);";
    
    sqlite3_stmt *stmt;
    int rc;
    
    rc = sqlite3_prepare_v2(g_db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "[ERROR] Error preparando statement: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    sqlite3_bind_text(stmt, 1, app_id, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 2, action, -1, SQLITE_STATIC);
    sqlite3_bind_text(stmt, 3, message, -1, SQLITE_STATIC);
    
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    
    if (rc != SQLITE_DONE) {
        fprintf(stderr, "[ERROR] Error guardando log: %s\n", sqlite3_errmsg(g_db));
        return -1;
    }
    
    return 0;
} 