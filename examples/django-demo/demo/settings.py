import os

import dj_database_url

BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# the module generates this per instance and injects it from a secret
SECRET_KEY = os.environ.get("SECRET_KEY", "insecure-demo-key")
DEBUG = False

# the module does not model ALLOWED_HOSTS yet, so accept anything: this image
# exists to exercise the deployment path, not to be run in production
ALLOWED_HOSTS = ["*"]

INSTALLED_APPS = [
    "django.contrib.contenttypes",
    "django.contrib.auth",
]

MIDDLEWARE = []
ROOT_URLCONF = "demo.urls"
WSGI_APPLICATION = "demo.wsgi.application"

# DATABASE_URL is what the module puts in the shared secret
DATABASES = {"default": dj_database_url.config(default=os.environ["DATABASE_URL"])}

USE_TZ = True
