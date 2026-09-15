from django.http import JsonResponse
from django.urls import path


def health(request):
    """Reports that the app started and can reach its database."""
    from django.db import connection

    with connection.cursor() as cursor:
        cursor.execute("SELECT 1")
        cursor.fetchone()

    return JsonResponse({"status": "ok", "database": "reachable"})


urlpatterns = [path("", health)]
