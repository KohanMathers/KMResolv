package me.kmathers.kmresolv;

import java.io.IOException;
import java.io.StringReader;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

import com.google.gson.JsonIOException;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.google.gson.JsonSyntaxException;
import com.google.gson.Strictness;
import com.google.gson.stream.JsonReader;

public class ApiClient {

    private static final HttpClient client = HttpClient.newHttpClient();
    private static String baseUrl = "http://127.0.0.1:8080";

    public static void setBaseUrl(String url) {
        baseUrl = url;
    }

    public static void post(String path) {
        post(path, "{}");
    }

    public static void post(String path, String body) {
        try {
            HttpRequest req = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + path))
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(body))
                .build();
            client.sendAsync(req, HttpResponse.BodyHandlers.discarding());
        } catch (Exception e) {
            System.err.println("API post failed: " + e.getMessage());
        }
    }

    public static JsonObject get(String path) {
        try {
            HttpRequest req = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + path))
                .GET()
                .build();
            HttpResponse<String> resp = client.send(req, HttpResponse.BodyHandlers.ofString());
            
            JsonReader reader = new JsonReader(new StringReader(resp.body()));
            reader.setStrictness(Strictness.LENIENT);
            return JsonParser.parseReader(reader).getAsJsonObject();
        } catch (JsonIOException | JsonSyntaxException | IOException | InterruptedException e) {
            System.err.println("API get failed: " + e.getMessage());
            return new JsonObject();
        }
    }
}