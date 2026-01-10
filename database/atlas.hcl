// Atlas Configuration File
// Configure database connection and migration settings

// Define the environment for local development
env "local" {
  // Database URL - replace with your actual database connection string
  url = "postgres://postgres:pass@localhost:5432/fotafoto?sslmode=disable"
  
  // Migration directory
  migration {
    dir = "file://database/migrations"
  }
  
  // Schema source
  src = "file://database/schemas/schema.sql"
}

// Define the environment for development
env "dev" {
  url = getenv("DATABASE_URL")
  
  migration {
    dir = "file://database/migrations"
  }
  
  src = "file://database/schemas/schema.sql"
}

// Define the environment for production
env "prod" {
  url = getenv("DATABASE_URL")
  
  migration {
    dir = "file://database/migrations"
  }
  
  src = "file://database/schemas/schema.sql"
  
  // Lint policies for production
  lint {
    destructive {
      error = true
    }
  }
}
