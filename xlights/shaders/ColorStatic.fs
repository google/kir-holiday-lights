/*{
  "ISFVSN": "2",
  "CATEGORIES": [ "xLights", "Generator" ],
  "DESCRIPTION": "Static random traditional colors with adjustable density (resolution-aware).",
  "INPUTS": [
    {
      "NAME": "density",
      "TYPE": "float",
      "DEFAULT": 800.0,
      "MIN": 10.0,
      "MAX": 800.0
    },
    {
      "NAME": "seed",
      "TYPE": "float",
      "DEFAULT": 1.0,
      "MIN": 1.0,
      "MAX": 1000.0
    }
  ]
}*/



// Bunch of random gibberish math to provide a "hash"
float hash(vec2 p, float seed) {
    vec2 q = p + vec2(seed, seed * 1.61803398875);
    float n = dot(q, vec2(3.14159265359, 2.71828182846)) + seed * 6.28318530718;
    return fract(sin(n) * 43758.5453123);

    return fract(p.x * p.y);
}

vec3 palette(float t) {
    // Rough guess at "traditional" light colors based on my local Italian restaurant's retro lights.
    vec3 red   = vec3(255.0, 0.0,   0.0)   / 255.0;
    vec3 green = vec3(0.0,   255.0, 0.0)   / 255.0;
    vec3 blue  = vec3(0.0,   0.0,   255.0) / 255.0;
    vec3 amber = vec3(255.0, 126.0, 1.0)   / 255.0;
    vec3 pink  = vec3(255.0, 66.0,  169.0) / 255.0;

    if (t < 0.20) return red;
    if (t < 0.40) return green;
    if (t < 0.60) return blue;
    if (t < 0.80) return amber;
    return pink;
}

void main() {
    vec2 uv = isf_FragNormCoord.xy;

    // Scale density relative to the render size
    float d = density * (max(RENDERSIZE.x, RENDERSIZE.y) / 512.0);
    d = max(d, 1.0);

    vec2 cell = floor(uv * d);

    float r = hash(cell, seed);

    gl_FragColor = vec4(palette(r), 1.0);
}

