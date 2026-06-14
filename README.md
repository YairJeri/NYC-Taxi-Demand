# Plataforma NeuroOps (ML Distribuido)

NeuroOps es una plataforma de Machine Learning distribuida diseñada para entrenar modelos predictivos (como pronósticos de demanda de taxis) utilizando una arquitectura Maestro-Trabajador (Parameter Server). Todo el ecosistema está orquestado mediante Docker y se compone de microservicios escritos en Go, junto con un panel de control interactivo en Astro y Preact.

## Arquitectura del Sistema

La plataforma está dividida en cuatro componentes principales:

1. **Frontend (Panel de Control)**
   - Construido con **Astro**, **Preact** y **Tailwind CSS**.
   - Proporciona una interfaz SPA (Single Page Application) moderna para monitorear el clúster de trabajadores (telemetría de CPU, RAM, pérdida de entrenamiento).
   - Permite despachar trabajos de procesamiento de datos y entrenamiento, así como ejecutar predicciones (inferencia) en tiempo real.
   - Orquestado a través de un servidor Nginx en producción para un enrutamiento adecuado.

2. **Backend API (Nodo Maestro)**
   - Desarrollado en **Go**.
   - Actúa como el orquestador del sistema. Mantiene un servidor TCP (`:9090`) donde los nodos trabajadores se conectan, reciben pesos del modelo y devuelven gradientes parciales (patrón Parameter Server).
   - Expone una API REST (`:8080`) para que el frontend interactúe (inicio de entrenamiento, consulta de estado, inferencia).
   - Coordina la limpieza de datos y la agregación en la base de datos central.

3. **ML Worker (Nodo de Cómputo)**
   - Desarrollado en **Go**.
   - Los trabajadores son clientes TCP puros que se conectan automáticamente al Nodo Maestro.
   - Realizan entrenamiento de Descenso de Gradiente Estocástico (SGD) paralelo.
   - Son horizontalmente escalables (puedes instanciar múltiples réplicas para acelerar el entrenamiento).

4. **Bases de Datos**
   - **MongoDB**: Almacena el dataset procesado (viajes) y mantiene el historial de entrenamientos y arquitecturas de modelo.
   - **Redis**: Actúa como un sistema de caché ultrarrápido para las inferencias del modelo, evitando recomputar consultas frecuentes y manteniendo la latencia de predicción en el mínimo.

## Características Principales

- **Entrenamiento Paralelo:** Arquitectura escalable y descentralizada de procesamiento de tensores usando redes neuronales profundas (Modelos Estáticos y Temporales).
- **Telemetría en Tiempo Real:** Monitorización en vivo del uso de CPU, RAM y reducción de pérdida (loss) por cada nodo conectado.
- **Tolerancia a Fallos:** Los trabajadores pueden conectarse y desconectarse dinámicamente; el Nodo Maestro redistribuirá las cargas de trabajo.
- **Diseño Responsive y Oscuro:** Interfaz limpia estilo SaaS para facilitar las operaciones (MLOps).

## Requisitos Previos

- [Docker](https://docs.docker.com/get-docker/) y [Docker Compose](https://docs.docker.com/compose/install/) instalados.

## Cómo Ejecutar (Despliegue con Docker)

La plataforma está dividida en dos archivos Compose para separar la infraestructura principal de los trabajadores. Ejecuta cada uno por separado:

```bash
# 1. Construir y levantar la infraestructura principal (API, Mongo, Redis, Frontend)
docker compose -f docker-compose.yml up --build -d

# 2. Levantar los nodos trabajadores (ML Workers)
docker compose -f docker-compose.worker.yml up --build -d
```

Una vez que los contenedores estén funcionando:

- **Frontend:** [http://localhost:3000](http://localhost:3000)
- **API REST (Maestro):** `http://localhost:8080`
- **Servidor TCP (Maestro):** `localhost:9090`
- **MongoDB:** `localhost:27017`
- **Redis:** `localhost:6379`

## Uso de la Plataforma

1. Abre el **Panel de Control** en tu navegador (`http://localhost:3000`).
2. Ve a la pestaña **Trabajos de Entrenamiento** y haz clic en "Iniciar Trabajo de Procesamiento (/clean)" para cargar y preparar el conjunto de datos en MongoDB.
3. Configura los hiperparámetros (Ej. Modelo Temporal, 5000 pasos) y despacha el trabajo.
4. Cambia a la vista **Nodos de Trabajo** para ver la telemetría en tiempo real mientras el modelo se optimiza.
5. Una vez terminado, dirígete a **Inferencia de Modelo** para probar predicciones (ej. predecir la demanda de taxis en una zona a una hora específica).

---

*Desarrollado como proyecto de Machine Learning Distribuido y MLOps.*
