# Diplo - PaaS Local en C
# Makefile para compilación en Raspberry Pi

# Compilador y flags
CC = gcc
CFLAGS = -Wall -Wextra -std=c99 -pedantic -g -O2

# Flags de compilación y enlazado según el sistema
ifeq ($(shell uname),Darwin)
    # macOS con Homebrew
    CFLAGS += -I/opt/homebrew/include
    LDFLAGS = -L/opt/homebrew/lib -lmicrohttpd -lcurl -ljansson -lsqlite3 -lpthread
else
    # Linux
    LDFLAGS = -lmicrohttpd -lcurl -ljansson -lsqlite3 -lpthread
endif

# Directorios
SRCDIR = src
INCDIR = include
LIBDIR = lib
BINDIR = bin
OBJDIR = obj

# Archivos fuente
SOURCES = $(wildcard $(SRCDIR)/*.c)
OBJECTS = $(SOURCES:$(SRCDIR)/%.c=$(OBJDIR)/%.o)

# Nombre del ejecutable
TARGET = $(BINDIR)/diplo

# Regla principal
all: $(TARGET)

# Compilar el ejecutable
$(TARGET): $(OBJECTS) | $(BINDIR)
	$(CC) $(OBJECTS) -o $@ $(LDFLAGS)

# Compilar objetos
$(OBJDIR)/%.o: $(SRCDIR)/%.c | $(OBJDIR)
	$(CC) $(CFLAGS) -I$(INCDIR) -c $< -o $@

# Crear directorios si no existen
$(BINDIR):
	mkdir -p $(BINDIR)

$(OBJDIR):
	mkdir -p $(OBJDIR)

# Limpiar
clean:
	rm -rf $(OBJDIR) $(BINDIR)

# Instalar (para sistema)
install: $(TARGET)
	sudo cp $(TARGET) /usr/local/bin/
	sudo chmod +x /usr/local/bin/diplo

# Desinstalar
uninstall:
	sudo rm -f /usr/local/bin/diplo

# Ejecutar
run: $(TARGET)
	./$(TARGET)

# Debug
debug: CFLAGS += -DDEBUG -g3
debug: $(TARGET)

# Verificar dependencias
check-deps:
	@echo "Verificando dependencias..."
	@which gcc > /dev/null || (echo "Error: gcc no encontrado" && exit 1)
	@echo "Sistema detectado: $(shell uname)"
	@if [ "$(shell uname)" = "Darwin" ]; then \
		echo "macOS detectado - usando Homebrew"; \
		brew list libmicrohttpd > /dev/null 2>&1 || (echo "Error: libmicrohttpd no encontrado. Instala con: brew install libmicrohttpd" && exit 1); \
		brew list curl > /dev/null 2>&1 || (echo "Error: curl no encontrado. Instala con: brew install curl" && exit 1); \
		brew list jansson > /dev/null 2>&1 || (echo "Error: jansson no encontrado. Instala con: brew install jansson" && exit 1); \
		brew list sqlite3 > /dev/null 2>&1 || (echo "Error: sqlite3 no encontrado. Instala con: brew install sqlite3" && exit 1); \
	else \
		echo "Linux detectado - usando pkg-config"; \
		which pkg-config > /dev/null || (echo "Error: pkg-config no encontrado" && exit 1); \
		pkg-config --exists libmicrohttpd || (echo "Error: libmicrohttpd no encontrado" && exit 1); \
		pkg-config --exists libcurl || (echo "Error: libcurl no encontrado" && exit 1); \
		pkg-config --exists jansson || (echo "Error: jansson no encontrado" && exit 1); \
		pkg-config --exists sqlite3 || (echo "Error: sqlite3 no encontrado" && exit 1); \
	fi
	@echo "Todas las dependencias están instaladas"

# Instalar dependencias (Raspberry Pi)
install-deps:
	@if [ "$(shell uname)" = "Darwin" ]; then \
		echo "Instalando dependencias en macOS..."; \
		brew install libmicrohttpd curl jansson sqlite3; \
	else \
		echo "Instalando dependencias en Linux..."; \
		sudo apt-get update; \
		sudo apt-get install -y libmicrohttpd-dev libcurl4-openssl-dev libjansson-dev libsqlite3-dev; \
	fi

.PHONY: all clean install uninstall run debug check-deps install-deps