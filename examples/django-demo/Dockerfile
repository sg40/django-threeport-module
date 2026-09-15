FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY manage.py .
COPY demo demo

# the module's deployment expects the app on 8000 and runs django-admin for
# migrations, both of which this satisfies
EXPOSE 8000
CMD ["gunicorn", "--bind", "0.0.0.0:8000", "demo.wsgi:application"]
