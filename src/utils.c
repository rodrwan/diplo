#include "diplo.h"
#include <time.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>

// Generar un ID único para la aplicación
char* diplo_generate_app_id(void) {
    static char app_id[64];
    time_t now = time(NULL);
    unsigned int random_num = (unsigned int)(now % 1000000);
    
    snprintf(app_id, sizeof(app_id), "app_%lu_%u", (unsigned long)now, random_num);
    return app_id;
}

// Encontrar un puerto libre en el rango especificado
int diplo_find_free_port(void) {
    // Rango de puertos para aplicaciones web (3000-9999)
    const int MIN_PORT = 3000;
    const int MAX_PORT = 9999;
    
    // Intentar puertos aleatorios en el rango
    for (int attempts = 0; attempts < 100; attempts++) {
        // Generar puerto aleatorio en el rango
        int port = MIN_PORT + (rand() % (MAX_PORT - MIN_PORT + 1));
        
        int sock = socket(AF_INET, SOCK_STREAM, 0);
        if (sock < 0) {
            continue;
        }
        
        // Configurar opciones para reutilizar puerto
        int opt = 1;
        setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
        
        struct sockaddr_in addr;
        memset(&addr, 0, sizeof(addr));
        addr.sin_family = AF_INET;
        addr.sin_addr.s_addr = htonl(INADDR_ANY);
        addr.sin_port = htons(port);
        
        if (bind(sock, (struct sockaddr*)&addr, sizeof(addr)) == 0) {
            close(sock);
            printf("[INFO] Puerto libre encontrado: %d\n", port);
            return port;
        }
        
        close(sock);
    }
    
    fprintf(stderr, "[ERROR] No se pudo encontrar puerto libre en el rango %d-%d\n", MIN_PORT, MAX_PORT);
    return -1;
}

// Ejecutar un comando del sistema
int diplo_exec_command(const char *command, char *output, size_t output_size) {
    FILE *fp = popen(command, "r");
    if (!fp) {
        return -1;
    }
    
    size_t bytes_read = fread(output, 1, output_size - 1, fp);
    output[bytes_read] = '\0';
    
    int status = pclose(fp);
    return WEXITSTATUS(status);
}

// Buscar una aplicación por ID
diplo_app_t* diplo_find_app(diplo_server_t *server, const char *app_id) {
    for (int i = 0; i < server->apps_count; i++) {
        if (strcmp(server->apps[i].id, app_id) == 0) {
            return &server->apps[i];
        }
    }
    return NULL;
}

// Verificar si un puerto específico está en uso
int diplo_is_port_in_use(int port) {
    int sock = socket(AF_INET, SOCK_STREAM, 0);
    if (sock < 0) {
        return 1; // Asumir que está en uso si no se puede crear socket
    }
    
    // Configurar opciones para reutilizar puerto
    int opt = 1;
    setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
    
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    addr.sin_port = htons(port);
    
    int result = bind(sock, (struct sockaddr*)&addr, sizeof(addr));
    close(sock);
    
    return (result != 0); // 1 si está en uso, 0 si está libre
} 